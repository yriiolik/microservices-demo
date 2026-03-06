// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/productcatalogservice/genproto"
	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"cloud.google.com/go/profiler"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
)

var (
	catalogMutex *sync.Mutex
	log          *logrus.Logger
	extraLatency time.Duration

	port = "3550"

	reloadCatalog bool

	mysqlDB *sql.DB
)

func init() {
	log = logrus.New()
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
	catalogMutex = &sync.Mutex{}
}

func main() {
	if os.Getenv("ENABLE_TRACING") == "1" {
		err := initTracing()
		if err != nil {
			log.Warnf("warn: failed to start tracer: %+v", err)
		}
	} else {
		log.Info("Tracing disabled.")
	}

	if os.Getenv("DISABLE_PROFILER") == "" {
		log.Info("Profiling enabled.")
		go initProfiling("productcatalogservice", "1.0.0")
	} else {
		log.Info("Profiling disabled.")
	}

	flag.Parse()

	// set injected latency
	if s := os.Getenv("EXTRA_LATENCY"); s != "" {
		v, err := time.ParseDuration(s)
		if err != nil {
			log.Fatalf("failed to parse EXTRA_LATENCY (%s) as time.Duration: %+v", v, err)
		}
		extraLatency = v
		log.Infof("extra latency enabled (duration: %v)", extraLatency)
	} else {
		extraLatency = time.Duration(0)
	}

	// Initialize MySQL connection if configured
	if mysqlAddr := os.Getenv("MYSQL_ADDR"); mysqlAddr != "" {
		mysqlUser := os.Getenv("MYSQL_USER")
		if mysqlUser == "" {
			mysqlUser = "boutique"
		}
		mysqlPassword := os.Getenv("MYSQL_PASSWORD")
		if mysqlPassword == "" {
			mysqlPassword = "boutique123"
		}
		mysqlDatabase := os.Getenv("MYSQL_DATABASE")
		if mysqlDatabase == "" {
			mysqlDatabase = "boutique"
		}
		dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&timeout=5s", mysqlUser, mysqlPassword, mysqlAddr, mysqlDatabase)

		log.Infof("Connecting to MySQL at %s...", mysqlAddr)
		var err error
		mysqlDB, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Fatalf("failed to open MySQL connection: %v", err)
		}
		mysqlDB.SetMaxOpenConns(10)
		mysqlDB.SetMaxIdleConns(5)
		mysqlDB.SetConnMaxLifetime(5 * time.Minute)

		// Retry ping until MySQL is ready (init scripts may take a few minutes)
		for i := 0; i < 60; i++ {
			err = mysqlDB.Ping()
			if err == nil {
				log.Info("Successfully connected to MySQL")
				break
			}
			log.Warnf("MySQL not ready (attempt %d/60): %v", i+1, err)
			time.Sleep(2 * time.Second)
		}
		if err != nil {
			log.Warnf("failed to connect to MySQL after retries: %v, falling back to local catalog", err)
			mysqlDB.Close()
			mysqlDB = nil
		}
	} else {
		log.Info("MYSQL_ADDR not set, using local product catalog")
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for {
			sig := <-sigs
			log.Printf("Received signal: %s", sig)
			if sig == syscall.SIGUSR1 {
				reloadCatalog = true
				log.Infof("Enable catalog reloading")
			} else {
				reloadCatalog = false
				log.Infof("Disable catalog reloading")
			}
		}
	}()

	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}
	log.Infof("starting grpc server at :%s", port)
	run(port)
	select {}
}

func run(port string) string {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatal(err)
	}

	// Propagate trace context
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{}))
	var srv *grpc.Server
	srv = grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()))

	svc := &productCatalog{}
	err = loadCatalog(&svc.catalog)
	if err != nil {
		log.Fatalf("could not parse product catalog: %v", err)
	}

	pb.RegisterProductCatalogServiceServer(srv, svc)
	healthcheck := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthcheck)
	go srv.Serve(listener)

	return listener.Addr().String()
}

func initStats() {
	// TODO(drewbr) Implement OpenTelemetry stats
}

func initTracing() error {
	var (
		collectorAddr string
		collectorConn *grpc.ClientConn
	)

	ctx := context.Background()

	mustMapEnv(&collectorAddr, "COLLECTOR_SERVICE_ADDR")
	mustConnGRPC(ctx, &collectorConn, collectorAddr)

	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithGRPCConn(collectorConn))
	if err != nil {
		log.Warnf("warn: Failed to create trace exporter: %v", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	return err
}

func initProfiling(service, version string) {
	for i := 1; i <= 3; i++ {
		if err := profiler.Start(profiler.Config{
			Service:        service,
			ServiceVersion: version,
			// ProjectID must be set if not running on GCP.
			// ProjectID: "my-project",
		}); err != nil {
			log.Warnf("failed to start profiler: %+v", err)
		} else {
			log.Info("started Stackdriver profiler")
			return
		}
		d := time.Second * 10 * time.Duration(i)
		log.Infof("sleeping %v to retry initializing Stackdriver profiler", d)
		time.Sleep(d)
	}
	log.Warn("could not initialize Stackdriver profiler after retrying, giving up")
}

func mustMapEnv(target *string, envKey string) {
	v := os.Getenv(envKey)
	if v == "" {
		panic(fmt.Sprintf("environment variable %q not set", envKey))
	}
	*target = v
}

func mustConnGRPC(ctx context.Context, conn **grpc.ClientConn, addr string) {
	var err error
	_, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()
	*conn, err = grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		panic(errors.Wrapf(err, "grpc: failed to connect %s", addr))
	}
}
