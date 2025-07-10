package main

import (
	"context"
	"log"
	"time"

	"orders/ent"
	"orders/handler"

	_ "github.com/mattn/go-sqlite3" // Import for SQLite driver

	"entgo.io/ent/dialect"
	"go-micro.dev/v5"
	"go-micro.dev/v5/logger"

	pb "orders/proto"
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
		micro.Name("orders"),
		micro.Version("latest"),
		micro.Metadata(map[string]string{
			"StartTime": time.Now().String(),
		}),
		micro.BeforeStart(func() error {
			logger.Info("Order service starting...")
			return nil
		}),
		micro.AfterStop(func() error {
			logger.Info("Order service stopped")
			return nil
		}),
	)

	// Initialize service
	service.Init()

	// Register OrderService handler
	if err := pb.RegisterOrderServiceHandler(service.Server(), &handler.OrderService{EntClient: client}); err != nil {
		logger.Fatalf("Failed to register order service handler: %v", err)
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
