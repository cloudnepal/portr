package service

import (
	"fmt"

	"github.com/amalshaji/localport/internal/server/db"
	"github.com/amalshaji/localport/internal/server/smtp"
	"github.com/oklog/ulid/v2"
	"github.com/valyala/fasttemplate"
)

func (s *Service) sendInviteEmail(invite *db.Invite) error {
	inviteURL := fmt.Sprintf("%s/invite/%s", s.config.AdminUrl(), invite.InviteUid)

	// get email template
	settings := s.ListSettings()

	t := fasttemplate.New(settings.UserInviteEmailTemplate, "{{", "}}")
	renderedText := t.ExecuteString(map[string]interface{}{
		"inviteUrl": inviteURL,
		"email":     invite.Email,
		"role":      invite.Role,
	})

	smtpInput := smtp.SendEmailInput{
		From:    s.config.Admin.Smtp.FromEmail,
		To:      invite.Email,
		Subject: "You have been invited to Localport",
		Body:    renderedText,
	}

	if err := s.smtp.SendEmail(smtpInput); err != nil {
		s.log.Error("failed to send invite email", "error", err)
		return err
	}
	return nil
}

func (s *Service) CreateInvite(input CreateInviteInput, invitedBy db.User) (*db.Invite, error) {
	// check if user exists
	var count int64
	s.db.Conn.Model(&db.User{}).Where("email = ?", input.Email).Count(&count)

	if count == 1 {
		return nil, fmt.Errorf("user with email %s already exists", input.Email)
	}

	// check if invite exists
	var invite db.Invite
	result := s.db.Conn.Where("email = ? AND status = ?", input.Email, db.Invited).First(&invite)
	if result.Error == nil {
		return nil, fmt.Errorf("the user is already invited")
	}

	tx := s.db.Conn.Begin()

	// create new invite
	invite = db.Invite{
		Email:         input.Email,
		Role:          db.UserRole(input.Role),
		InvitedByUser: invitedBy,
		InviteUid:     ulid.Make().String(),
	}

	result = tx.Create(&invite)
	if result.Error != nil {
		tx.Rollback()
		return nil, result.Error
	}

	// send invite email
	if err := s.sendInviteEmail(&invite); err != nil {
		tx.Rollback()
		return nil, err
	}

	tx.Commit()
	return &invite, nil
}

func (s *Service) ListInvites() []db.Invite {
	var invites []db.Invite
	s.db.Conn.Joins("InvitedByUser").Find(&invites)
	return invites
}