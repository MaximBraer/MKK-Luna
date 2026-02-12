//go:build integration

package integration

import (
	"context"
	"errors"
)

type emailOKSender struct{}

func (emailOKSender) SendInvite(_ context.Context, _ string, _ string) error { return nil }

type emailFailSender struct{}

func (emailFailSender) SendInvite(_ context.Context, _ string, _ string) error {
	return errors.New("email down")
}
