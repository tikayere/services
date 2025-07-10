package main

import (
	"context"
	"log"
	"time"
	"users/ent"
	"users/handler"

	_ "github.com/mattn/go-sqlite3" // Import for SQLite driver

	"entgo.io/ent/dialect"
	"go-micro.dev/v5"
	"go-micro.dev/v5/logger"

	pb "users/proto"
)

func main() {
	// Initialize EntgoClient
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		logger.Fatalf("Failed opening connection to sqlite: %v", err)
	}
	defer client.Close()

	// Run the auto migration tool. This will create table and columns in the database
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("Failed creating schema resources: %v", err)
	}

	// Create a new service
	service := micro.NewService(
		micro.Name("users"),
		micro.Version("latest"),
		micro.Metadata(map[string]string{
			"StartTime": time.Now().String(),
		}),
		micro.BeforeStart(func() error {
			logger.Info("Server service starting...")
			return nil
		}),
		micro.AfterStop(func() error {
			logger.Info("User service stopped")
			return nil
		}),
	)

	// Initialize service
	service.Init()

	// Register UserService handler
	if err := pb.RegisterUserServiceHandler(service.Server(), &handler.User{EntClient: client}); err != nil {
		logger.Fatalf("failed to register user service handler: %v", err)
	}

	if err := pb.RegisterAdminServiceHandler(service.Server(), &handler.AdminService{EntClient: client}); err != nil {
		logger.Fatalf("failed to register admin service handler: %v", err)
	}

	// Run the service
	if err := service.Run(); err != nil {
		logger.Fatalf("failed to run service: %v", err)
	}
}
