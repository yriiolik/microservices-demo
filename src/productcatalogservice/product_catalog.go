// Copyright 2023 Google LLC
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
	"fmt"
	"strings"
	"time"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/productcatalogservice/genproto"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type productCatalog struct {
	pb.UnimplementedProductCatalogServiceServer
	catalog pb.ListProductsResponse
}

func (p *productCatalog) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (p *productCatalog) Watch(req *healthpb.HealthCheckRequest, ws healthpb.Health_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "health check via Watch not implemented")
}

func (p *productCatalog) ListProducts(ctx context.Context, req *pb.Empty) (*pb.ListProductsResponse, error) {
	time.Sleep(extraLatency)

	if mysqlDB != nil {
		products, err := listProductsFromMySQL()
		if err != nil {
			log.Warnf("MySQL ListProducts failed, falling back to catalog: %v", err)
			return &pb.ListProductsResponse{Products: p.parseCatalog()}, nil
		}
		return &pb.ListProductsResponse{Products: products}, nil
	}

	return &pb.ListProductsResponse{Products: p.parseCatalog()}, nil
}

func (p *productCatalog) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.Product, error) {
	time.Sleep(extraLatency)

	if mysqlDB != nil {
		product, err := getProductFromMySQL(req.Id)
		if err != nil {
			log.Warnf("MySQL GetProduct failed, falling back to catalog: %v", err)
		} else if product != nil {
			return product, nil
		}
	}

	var found *pb.Product
	for i := 0; i < len(p.parseCatalog()); i++ {
		if req.Id == p.parseCatalog()[i].Id {
			found = p.parseCatalog()[i]
		}
	}

	if found == nil {
		return nil, status.Errorf(codes.NotFound, "no product with ID %s", req.Id)
	}
	return found, nil
}

func (p *productCatalog) SearchProducts(ctx context.Context, req *pb.SearchProductsRequest) (*pb.SearchProductsResponse, error) {
	time.Sleep(extraLatency)

	if mysqlDB != nil {
		products, err := searchProductsInMySQL(req.Query)
		if err != nil {
			log.Warnf("MySQL SearchProducts failed, falling back to catalog: %v", err)
		} else {
			return &pb.SearchProductsResponse{Results: products}, nil
		}
	}

	var ps []*pb.Product
	for _, product := range p.parseCatalog() {
		if strings.Contains(strings.ToLower(product.Name), strings.ToLower(req.Query)) ||
			strings.Contains(strings.ToLower(product.Description), strings.ToLower(req.Query)) {
			ps = append(ps, product)
		}
	}

	return &pb.SearchProductsResponse{Results: ps}, nil
}

func (p *productCatalog) parseCatalog() []*pb.Product {
	if reloadCatalog || len(p.catalog.Products) == 0 {
		err := loadCatalog(&p.catalog)
		if err != nil {
			return []*pb.Product{}
		}
	}

	return p.catalog.Products
}

// MySQL query functions

func listProductsFromMySQL() ([]*pb.Product, error) {
	rows, err := mysqlDB.Query("SELECT id, name, description, picture, price_usd_currency_code, price_usd_units, price_usd_nanos FROM products")
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var products []*pb.Product
	for rows.Next() {
		p := &pb.Product{PriceUsd: &pb.Money{}}
		if err := rows.Scan(&p.Id, &p.Name, &p.Description, &p.Picture,
			&p.PriceUsd.CurrencyCode, &p.PriceUsd.Units, &p.PriceUsd.Nanos); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		cats, err := getCategoriesForProduct(p.Id)
		if err != nil {
			log.Warnf("failed to get categories for product %s: %v", p.Id, err)
			p.Categories = []string{}
		} else {
			p.Categories = cats
		}
		products = append(products, p)
	}
	return products, nil
}

func getProductFromMySQL(id string) (*pb.Product, error) {
	p := &pb.Product{PriceUsd: &pb.Money{}}
	err := mysqlDB.QueryRow(
		"SELECT id, name, description, picture, price_usd_currency_code, price_usd_units, price_usd_nanos FROM products WHERE id = ?", id,
	).Scan(&p.Id, &p.Name, &p.Description, &p.Picture,
		&p.PriceUsd.CurrencyCode, &p.PriceUsd.Units, &p.PriceUsd.Nanos)
	if err != nil {
		return nil, fmt.Errorf("query product %s: %w", id, err)
	}
	cats, err := getCategoriesForProduct(p.Id)
	if err != nil {
		p.Categories = []string{}
	} else {
		p.Categories = cats
	}
	return p, nil
}

func searchProductsInMySQL(query string) ([]*pb.Product, error) {
	likeQuery := "%" + strings.ToLower(query) + "%"
	rows, err := mysqlDB.Query(
		"SELECT id, name, description, picture, price_usd_currency_code, price_usd_units, price_usd_nanos FROM products WHERE LOWER(name) LIKE ? OR LOWER(description) LIKE ?",
		likeQuery, likeQuery,
	)
	if err != nil {
		return nil, fmt.Errorf("search products: %w", err)
	}
	defer rows.Close()

	var products []*pb.Product
	for rows.Next() {
		p := &pb.Product{PriceUsd: &pb.Money{}}
		if err := rows.Scan(&p.Id, &p.Name, &p.Description, &p.Picture,
			&p.PriceUsd.CurrencyCode, &p.PriceUsd.Units, &p.PriceUsd.Nanos); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		cats, err := getCategoriesForProduct(p.Id)
		if err != nil {
			p.Categories = []string{}
		} else {
			p.Categories = cats
		}
		products = append(products, p)
	}
	return products, nil
}

func getCategoriesForProduct(productID string) ([]string, error) {
	rows, err := mysqlDB.Query(
		"SELECT c.name FROM categories c INNER JOIN product_categories pc ON c.id = pc.category_id WHERE pc.product_id = ?",
		productID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		categories = append(categories, name)
	}
	return categories, nil
}
