package main

import (
	"context"
	"log"
	"time"

	"carts/ent"
	"carts/handler"

	_ "github.com/mattn/go-sqlite3" // Import for SQLite driver

	"entgo.io/ent/dialect"
	"go-micro.dev/v5"
	"go-micro.dev/v5/logger"

	pb "carts/proto"
)

func main() {
	// Initialize EntgoClient
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		logger.Fatalf("Failed opening connection to sqlite: %v", err)
	}
	defer client.Close()

	// Run the auto migration tool
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("Failed creating schema resources: %v", err)
	}

	// Create a new service
	service := micro.NewService(
		micro.Name("carts"),
		micro.Version("latest"),
		micro.Metadata(map[string]string{
			"StartTime": time.Now().String(),
		}),
		micro.BeforeStart(func() error {
			logger.Info("Cart service starting...")
			return nil
		}),
		micro.AfterStop(func() error {
			logger.Info("Cart service stopped")
			return nil
		}),
	)

	// Initialize service
	service.Init()

	// Register CartService handler
	if err := pb.RegisterCartServiceHandler(service.Server(), &handler.CartService{EntClient: client}); err != nil {
		logger.Fatalf("Failed to register cart service handler: %v", err)
	}

	// Register AdminService handler
	if err := pb.RegisterAdminServiceHandler(service.Server(), &handler.AdminService{EntClient: client}); err != nil {
		logger.Fatalf("Failed to register admin service handler: %v", err)
	}

	// Run the service
	if err := service.Run(); err != nil {
		logger.Fatalf("Failed to run service: %v", err)
	}
}
