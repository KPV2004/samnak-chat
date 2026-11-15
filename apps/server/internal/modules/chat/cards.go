package chat

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// errCards is returned by every card operation until M2-05 implements them.
var errCards = fmt.Errorf("cards arrive in M2-05: %w", app.ErrInvalid)

// cardStub is the app.CardRegistry chat provides so the contract is wired.
// Every operation fails with an error wrapping app.ErrInvalid.
type cardStub struct{}

var _ app.CardRegistry = cardStub{}

func (cardStub) Register(app.CardKind) {}

func (cardStub) Post(context.Context, app.Card) (string, error) { return "", errCards }

func (cardStub) Update(context.Context, string, string, any) error { return errCards }

func (cardStub) DirectChannel(context.Context, string, []string) (string, error) {
	return "", errCards
}
