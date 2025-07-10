package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	log "go-micro.dev/v5/logger"
	"golang.org/x/crypto/bcrypt"

	"users/ent"
	"users/ent/user"
	pb "users/proto"
)

// User implements the UserServer interface
type User struct {
	EntClient *ent.Client
}

// CreateUser handles the creation of a new user
func (h *User) CreateUser(ctx context.Context, req *pb.CreateUserRequest, rsp *pb.CreateUserResponse) error {
	log.Infof("Received CreateUser request from username: %s, email: %s", req.Username, req.Email)

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Infof("Error hashing password: %v", err)
		return err
	}

	// Create user using Entgo
	u, err := h.EntClient.User.Create().
		SetEmail(req.Email).
		SetUsername(req.Username).
		SetPasswordHash(string(hashedPassword)).
		Save(ctx)
	if ent.IsConstraintError(err) {
		log.Errorf("Contraint violation: %v", err)
		return err
	}
	if err != nil {
		log.Errorf("Failed to create user: %v", err)
		return err
	}
	rsp.User = toProtoUser(u)
	log.Infof("User created successfully: %s", u.ID)
	return nil
}

// GetUser handles fetching a user by ID
func (h *User) GetUser(ctx context.Context, req *pb.GetUserRequest, rsp *pb.GetUserResponse) error {
	log.Infof("Received GetUser request for ID: %s", req.Id)

	u, err := h.EntClient.User.Get(ctx, uuid.MustParse(req.Id))
	if ent.IsNotFound(err) {
		log.Infof("User not found: %s", req.Id)
		return err
	}
	if err != nil {
		log.Infof("Failed to get user: %v", err)
		return err
	}

	rsp.User = toProtoUser(u)
	log.Infof("User fetched successfully: %s", u.ID)
	return nil
}

// UpdateUser handles updating an existing user
func (h *User) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest, rsp *pb.UpdateUserResponse) error {
	log.Infof("Received UpdateUser request for ID: %s", req.Id)

	updater := h.EntClient.User.UpdateOneID(uuid.MustParse(req.Id))

	if req.Email != "" {
		updater.Mutation().SetEmail(req.Email)
	}
	if req.Username != "" {
		updater.SetUsername(req.Username)
	}

	u, err := updater.Save(ctx)
	if ent.IsNotFound(err) {
		log.Infof("User not found for update: %s", req.Id)
		return err
	}
	if ent.IsConstraintError(err) {
		log.Infof("Constraint violation during update: %v", err)
		return err
	}
	if err != nil {
		log.Infof("Failed to update user: %v", err)
		return err
	}

	rsp.User = toProtoUser(u)
	log.Infof("User updated successfully: %s", u.ID)
	return nil
}

// ListUsers handles listing all users with optional pagination
func (h *User) ListUsers(ctx context.Context, req *pb.ListUsersRequest, rsp *pb.ListUsersResponse) error {
	log.Infof("Received ListUsers request (limit: %d, offset: %d)", req.Limit, req.Offset)

	query := h.EntClient.User.Query()

	// Apply pagination
	if req.Limit > 0 {
		query.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		query.Offset(int(req.Offset))
	}

	users, err := query.All(ctx)
	if err != nil {
		log.Infof("Failed to list users: %v", err)
		return err
	}

	total, err := h.EntClient.User.Query().Count(ctx)
	if err != nil {
		log.Infof("Failed to count users: %v", err)
		return err
	}

	protoUsers := make([]*pb.User, len(users))
	for i, u := range users {
		protoUsers[i] = toProtoUser(u)
	}

	rsp.Users = protoUsers
	rsp.Total = int32(total)
	log.Infof("Listed %d users (total: %d)", len(protoUsers), total)
	return nil
}

// Authenticate authenticates a user by email or username and password
func (h *User) Authenticate(ctx context.Context, req *pb.AuthenticateRequest, rsp *pb.AuthenticateResponse) error {
	log.Info("Received Authenticate request for: %s", req.EmailOrUsername)

	var u *ent.User
	var err error

	// Try to find user by email first, then by username
	u, err = h.EntClient.User.Query().
		Where(user.Email(req.EmailOrUsername)).
		WithProfile().
		Only(ctx)
	if ent.IsNotFound(err) {
		u, err = h.EntClient.User.Query().
			Where(user.Username(req.EmailOrUsername)).
			WithProfile().
			Only(ctx)
	}

	if ent.IsNotFound(err) {
		log.Info("Authentication failed: User not found for %s", req.EmailOrUsername)
		return fmt.Errorf("invalid credentials: user not found")
	}
	if err != nil {
		log.Info("Failed to query user for authentication: %v", err)
		return fmt.Errorf("internal server error during authentication: %w", err)
	}

	// Compare provided password with hashed password
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		log.Info("Authentication failed: Invalid password for user %s", u.ID)
		return fmt.Errorf("invalid credentials: incorrect password")
	}

	// Check if user is active
	if !u.IsActive {
		log.Info("Authentication failed: User %s is inactive", u.ID)
		return fmt.Errorf("user account is inactive")
	}

	// Check if email is verified (optional, but good practice for production)
	if !u.EmailVerified {
		log.Info("Authentication failed: User %s email not verified", u.ID)
		return fmt.Errorf("email not verified")
	}

	// For production, generate a proper JWT or session token here
	// For now, a dummy token
	token := fmt.Sprintf("dummy_token_%s_%d", u.ID, time.Now().Unix())

	rsp.User = toProtoUser(u)
	rsp.Token = token
	log.Info("User %s authenticated successfully. Token: %s", u.ID, token)
	return nil
}

// ChangePassword allows a user to change their password
func (h *User) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest, rsp *pb.ChangePasswordResponse) error {
	log.Info("Received ChangePassword request for user ID: %s", req.UserId)

	u, err := h.EntClient.User.Query().Where(user.ID(uuid.MustParse(req.UserId))).Only(ctx)
	if ent.IsNotFound(err) {
		log.Info("ChangePassword failed: User not found for ID %s", req.UserId)
		return fmt.Errorf("user not found")
	}
	if err != nil {
		log.Info("Failed to get user for password change: %v", err)
		return fmt.Errorf("internal server error: %w", err)
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		log.Info("ChangePassword failed: Incorrect old password for user %s", req.UserId)
		rsp.Success = false
		return fmt.Errorf("incorrect old password")
	}

	// Hash new password
	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Info("Error hashing new password: %v", err)
		rsp.Success = false
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// Update password hash
	_, err = h.EntClient.User.UpdateOneID(uuid.MustParse(req.UserId)).
		SetPasswordHash(string(newHashedPassword)).
		Save(ctx)
	if err != nil {
		log.Info("Failed to update password for user %s: %v", u.ID, err)
		rsp.Success = false
		return fmt.Errorf("failed to change password: %w", err)
	}

	rsp.Success = true
	log.Info("Password changed successfully for user: %s", req.UserId)
	return nil
}

// ResetPassword initiates a password reset flow (in a real app, sends email)
func (h *User) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest, rsp *pb.ResetPasswordResponse) error {
	log.Info("Received ResetPassword request for email: %s", req.Email)

	u, err := h.EntClient.User.Query().Where(user.Email(req.Email)).Only(ctx)
	if ent.IsNotFound(err) {
		// Log but don't expose if user not found to prevent enumeration attacks
		log.Info("ResetPassword request for non-existent email: %s", req.Email)
		rsp.Success = true // Still return success to avoid leaking user existence
		return nil
	}
	if err != nil {
		log.Info("Failed to get user for password reset: %v", err)
		return fmt.Errorf("internal server error: %w", err)
	}

	// In a real application:
	// 1. Generate a unique, time-limited password reset token.
	resetToken := uuid.New().String()
	// 2. Store this token and its expiry in the database (e.g., in a separate table or on the User schema).
	_, err = h.EntClient.User.UpdateOneID(u.ID).
		SetVerificationToken(resetToken). // Reusing verification_token for simplicity
		Save(ctx)
	if err != nil {
		log.Info("Failed to save reset token for user %s: %v", u.ID, err)
		return fmt.Errorf("failed to initiate password reset: %w", err)
	}
	// 3. Send an email to req.Email with a link containing this resetToken.
	log.Info("Password reset initiated for %s. Reset token: %s (in a real app, send via email)", req.Email, resetToken)

	rsp.Success = true
	return nil
}

// VerifyEmail verifies a user's email using a token
func (h *User) VerifyEmail(ctx context.Context, req *pb.VerifyEmailRequest, rsp *pb.VerifyEmailResponse) error {
	log.Info("Received VerifyEmail request with token.")

	u, err := h.EntClient.User.Query().Where(user.VerificationToken(req.Token)).Only(ctx)
	if ent.IsNotFound(err) {
		log.Info("Email verification failed: Invalid or expired token.")
		rsp.Success = false
		return fmt.Errorf("invalid or expired verification token")
	}
	if err != nil {
		log.Info("Failed to query user for email verification: %v", err)
		rsp.Success = false
		return fmt.Errorf("internal server error during email verification: %w", err)
	}

	if u.EmailVerified {
		log.Info("Email for user %s is already verified.", u.ID)
		rsp.Success = true
		return nil // Already verified, idempotent
	}

	// Mark email as verified and clear the token
	_, err = h.EntClient.User.UpdateOneID(u.ID).
		SetEmailVerified(true).
		ClearVerificationToken(). // Clear the token after use
		Save(ctx)
	if err != nil {
		log.Info("Failed to update email verification status for user %s: %v", u.ID, err)
		rsp.Success = false
		return fmt.Errorf("failed to verify email: %w", err)
	}

	rsp.Success = true
	log.Info("Email verified successfully for user: %s", u.ID)
	return nil
}

// GetUserByEmail gets a user by their email address
func (h *User) GetUserByEmail(ctx context.Context, req *pb.GetUserByEmailRequest, rsp *pb.GetUserResponse) error {
	log.Info("Received GetUserByEmail request for email: %s", req.Email)

	u, err := h.EntClient.User.Query().
		Where(user.Email(req.Email)).
		WithProfile().
		Only(ctx)
	if ent.IsNotFound(err) {
		log.Info("User not found for email: %s", req.Email)
		return fmt.Errorf("user not found")
	}
	if err != nil {
		log.Info("Failed to get user by email: %v", err)
		return fmt.Errorf("failed to get user by email: %w", err)
	}

	rsp.User = toProtoUser(u)
	log.Info("User fetched by email successfully: %s", u.ID)
	return nil
}

// GetUserByUsername gets a user by their username
func (h *User) GetUserByUsername(ctx context.Context, req *pb.GetUserByUsernameRequest, rsp *pb.GetUserResponse) error {
	log.Info("Received GetUserByUsername request for username: %s", req.Username)

	u, err := h.EntClient.User.Query().
		Where(user.Username(req.Username)).
		WithProfile().
		Only(ctx)
	if ent.IsNotFound(err) {
		log.Info("User not found for username: %s", req.Username)
		return fmt.Errorf("user not found")
	}
	if err != nil {
		log.Info("Failed to get user by username: %v", err)
		return fmt.Errorf("failed to get user by username: %w", err)
	}

	rsp.User = toProtoUser(u)
	log.Info("User fetched by username successfully: %s", u.ID)
	return nil
}

// SearchUsers searches users by query string (username or email)
func (h *User) SearchUsers(ctx context.Context, req *pb.SearchUsersRequest, rsp *pb.SearchUsersResponse) error {
	log.Info("Received SearchUsers request (query: %s, limit: %d, offset: %d)", req.Query, req.Limit, req.Offset)

	queryBuilder := h.EntClient.User.Query().WithProfile()

	if req.Query != "" {
		searchStr := "%" + req.Query + "%"
		queryBuilder.Where(user.Or(
			user.UsernameContainsFold(searchStr),
			user.EmailContainsFold(searchStr),
		))
	}

	// Apply pagination
	if req.Limit > 0 {
		queryBuilder.Limit(int(req.Limit))
	}
	if req.Offset > 0 {
		queryBuilder.Offset(int(req.Offset))
	}

	users, err := queryBuilder.All(ctx)
	if err != nil {
		log.Info("Failed to search users: %v", err)
		return fmt.Errorf("failed to search users: %w", err)

	}

	total, err := h.EntClient.User.Query().Count(ctx) // Count without limit/offset
	if err != nil {
		log.Info("Failed to count users for search: %v", err)
		return fmt.Errorf("failed to count users for search: %w", err)
	}

	protoUsers := make([]*pb.User, len(users))
	for i, u := range users {
		protoUsers[i] = toProtoUser(u)
	}

	rsp.Users = protoUsers
	rsp.Total = int32(total)
	log.Info("Found %d users matching query '%s' (total: %d)", len(protoUsers), req.Query, total)
	return nil
}

// toProtoUser converts an Entgo User entity to a Protobuf User message
func toProtoUser(u *ent.User) *pb.User {
	if u == nil {
		return nil
	}
	return &pb.User{
		Id:           u.ID.String(),
		Email:        u.Email,
		Username:     u.Username,
		PasswordHash: u.PasswordHash, // Be cautious: only send this if absolutely necessary and securely.
		CreatedAt:    u.CreatedAt.Unix(),
		UpdatedAt:    u.UpdatedAt.Unix(),
		IsActive:     u.IsActive,
	}
}
