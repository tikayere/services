// handler/admin_service.go
package handler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"users/ent"
	"users/ent/user" // Import user entity for eager loading
	pb "users/proto" // Import protobuf generated code
)

// AdminService implements the AdminServiceServer interface
type AdminService struct {
	EntClient *ent.Client // Entgo client instance
}

// ForceDeleteUser handles the forced deletion of a user (admin privilege)
func (h *AdminService) ForceDeleteUser(ctx context.Context, req *pb.ForceDeleteUserRequest, rsp *pb.ForceDeleteUserResponse) error {
	log.Printf("Received ForceDeleteUser request for ID: %s (Admin operation)", req.Id)

	// Start a transaction to ensure atomicity of user and profile deletion
	tx, err := h.EntClient.Tx(ctx)
	if err != nil {
		log.Printf("Failed to start transaction for force delete: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if an error occurs

	// Find the user to get their profile ID (if it exists)
	u, err := tx.User.Query().Where(user.ID(uuid.MustParse(req.Id))).WithProfile().Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		log.Printf("Failed to query user for force delete: %v", err)
		return fmt.Errorf("failed to query user: %w", err)
	}

	if u != nil && u.Edges.Profile != nil {
		// Delete the associated profile first due to foreign key constraints if not CASCADE
		err = tx.Profile.DeleteOneID(u.Edges.Profile.ID).Exec(ctx)
		if err != nil {
			log.Printf("Failed to delete profile for user %s: %v", req.Id, err)
			return fmt.Errorf("failed to delete profile: %w", err)
		}
		log.Printf("Profile for user %s deleted successfully.", req.Id)
	}

	// Now delete the user
	err = tx.User.DeleteOneID(uuid.MustParse(req.Id)).Exec(ctx)
	if ent.IsNotFound(err) {
		log.Printf("User not found for forced deletion: %s", req.Id)
		rsp.Success = false
		return fmt.Errorf("user not found for deletion: %w", err)
	}
	if err != nil {
		log.Printf("Failed to force delete user: %v", err)
		rsp.Success = false
		return fmt.Errorf("failed to force delete user: %w", err)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction for force delete: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	rsp.Id = req.Id
	rsp.Success = true
	log.Printf("User force deleted successfully: %s", req.Id)
	return nil
}

// SuspendUser handles suspending a user by setting is_active to false (admin privilege)
func (h *AdminService) SuspendUser(ctx context.Context, req *pb.SuspendUserRequest, rsp *pb.SuspendUserResponse) error {
	log.Printf("Received SuspendUser request for ID: %s (Admin operation)", req.Id)

	u, err := h.EntClient.User.UpdateOneID(uuid.MustParse(req.Id)).
		SetIsActive(false).
		Save(ctx)
	if ent.IsNotFound(err) {
		log.Printf("User not found for suspension: %s", req.Id)
		return fmt.Errorf("user not found for suspension: %w", err)
	}
	if err != nil {
		log.Printf("Failed to suspend user: %v", err)
		return fmt.Errorf("failed to suspend user: %w", err)
	}

	// Re-query user with profile to return complete user object
	uWithProfile, err := h.EntClient.User.Query().Where(user.ID(u.ID)).WithProfile().Only(ctx)
	if err != nil {
		log.Printf("Failed to retrieve user with profile after suspension: %v", err)
		return fmt.Errorf("failed to retrieve user after suspension: %w", err)
	}

	rsp.User = toProtoUser(uWithProfile)
	log.Printf("User suspended successfully: %s", u.ID)
	return nil
}

// ActivateUser handles activating a user by setting is_active to true (admin privilege)
func (h *AdminService) ActivateUser(ctx context.Context, req *pb.ActivateUserRequest, rsp *pb.ActivateUserResponse) error {
	log.Printf("Received ActivateUser request for ID: %s (Admin operation)", req.Id)

	u, err := h.EntClient.User.UpdateOneID(uuid.MustParse(req.Id)).
		SetIsActive(true).
		Save(ctx)
	if ent.IsNotFound(err) {
		log.Printf("User not found for activation: %s", req.Id)
		return fmt.Errorf("user not found for activation: %w", err)
	}
	if err != nil {
		log.Printf("Failed to activate user: %v", err)
		return fmt.Errorf("failed to activate user: %w", err)
	}

	// Re-query user with profile to return complete user object
	uWithProfile, err := h.EntClient.User.Query().Where(user.ID(u.ID)).WithProfile().Only(ctx)
	if err != nil {
		log.Printf("Failed to retrieve user with profile after activation: %v", err)
		return fmt.Errorf("failed to retrieve user after activation: %w", err)
	}

	rsp.User = toProtoUser(uWithProfile)
	log.Printf("User activated successfully: %s", u.ID)
	return nil
}

// BulkCreateUsers handles streaming creation of multiple users
func (h *AdminService) BulkCreateUsers(ctx context.Context, stream pb.AdminService_BulkCreateUsersStream) error {
	log.Printf("Received BulkCreateUsers stream request (Admin operation)")
	var createdUsers []*pb.User
	var totalCreated int32

	for {
		req := &pb.CreateUserRequest{}
		err := stream.RecvMsg(req)
		if err != nil {
			if err == fmt.Errorf("EOF") { // go-micro uses EOF for end of stream
				break
			}
			log.Printf("Error receiving from BulkCreateUsers stream: %v", err)
			return fmt.Errorf("error receiving user data: %w", err)
		}

		log.Printf("Bulk creating user: %s (email: %s)", req.Username, req.Email)

		// Hash the password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("BulkCreateUsers: Error hashing password for %s: %v", req.Username, err)
			// Continue with other users, but log the error
			continue
		}

		// Generate a verification token for email verification
		verificationToken := uuid.New().String()

		// Start a transaction for each user creation to ensure atomicity
		tx, err := h.EntClient.Tx(ctx)
		if err != nil {
			log.Printf("BulkCreateUsers: Failed to start transaction for %s: %v", req.Username, err)
			continue
		}

		u, err := tx.User.
			Create().
			SetEmail(req.Email).
			SetUsername(req.Username).
			SetPasswordHash(string(hashedPassword)).
			SetVerificationToken(verificationToken).
			SetEmailVerified(false).
			Save(ctx)

		if ent.IsConstraintError(err) {
			log.Printf("BulkCreateUsers: Constraint violation for user %s: %v", req.Username, err)
			tx.Rollback()
			continue
		}
		if err != nil {
			log.Printf("BulkCreateUsers: Failed to create user %s: %v", req.Username, err)
			tx.Rollback()
			continue
		}

		profileCreator := tx.Profile.Create().SetUser(u)
		if req.FirstName != "" {
			profileCreator.SetFirstName(req.FirstName)
		}
		if req.LastName != "" {
			profileCreator.SetLastName(req.LastName)
		}
		if req.DateOfBirth > 0 {
			profileCreator.SetDateOfBirth(time.Unix(req.DateOfBirth, 0))
		}
		if req.Address != "" {
			profileCreator.SetAddress(req.Address)
		}
		if req.PhoneNumber != "" {
			profileCreator.SetPhoneNumber(req.PhoneNumber)
		}

		_, err = profileCreator.Save(ctx)
		if err != nil {
			log.Printf("BulkCreateUsers: Failed to create profile for user %s: %v", u.ID, err)
			tx.Rollback()
			continue
		}

		if err = tx.Commit(); err != nil {
			log.Printf("BulkCreateUsers: Failed to commit transaction for user %s: %v", u.ID, err)
			continue
		}

		uWithProfile, err := h.EntClient.User.Query().Where(user.ID(u.ID)).WithProfile().Only(ctx)
		if err != nil {
			log.Printf("BulkCreateUsers: Failed to retrieve user with profile after creation %s: %v", u.ID, err)
			continue
		}

		createdUsers = append(createdUsers, toProtoUser(uWithProfile))
		totalCreated++
	}

	// Send the final response containing all created users
	err := stream.SendMsg(&pb.ListUsersResponse{
		Users: createdUsers,
		Total: totalCreated,
	})
	if err != nil {
		log.Printf("Error sending BulkCreateUsers response: %v", err)
		return fmt.Errorf("failed to send response: %w", err)
	}

	log.Printf("BulkCreateUsers: Successfully created %d users.", totalCreated)
	return nil
}

// ExportUsers streams all users, optionally filtered and paginated
func (h *AdminService) ExportUsers(ctx context.Context, req *pb.ListUsersRequest, stream pb.AdminService_ExportUsersStream) error {
	log.Printf("Received ExportUsers stream request (Admin operation) (limit: %d, offset: %d, filter: %s)", req.Limit, req.Offset, req.Filter)

	query := h.EntClient.User.Query().WithProfile() // Eager load profiles

	// Apply filter if provided
	if req.Filter != "" {
		filter := "%" + req.Filter + "%"
		query.Where(user.Or(
			user.UsernameContainsFold(filter),
			user.EmailContainsFold(filter),
		))
	}

	// Apply pagination (optional, but good for large datasets)
	if req.Limit > 0 {
		query.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		query.Offset(int(req.Offset))
	}

	users, err := query.All(ctx)
	if err != nil {
		log.Printf("Failed to retrieve users for export: %v", err)
		return fmt.Errorf("failed to retrieve users for export: %w", err)
	}

	for _, u := range users {
		protoUser := toProtoUser(u)
		if err := stream.Send(protoUser); err != nil {
			log.Printf("Error sending user %s during export: %v", u.ID, err)
			return fmt.Errorf("failed to stream user: %w", err)
		}
	}

	log.Printf("Successfully exported %d users.", len(users))
	return nil
}
