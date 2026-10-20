package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/inbound"
	"github.com/Anonyfox/lfr/internal/mailauth"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/outbound"
)

// localFirst delivers mail for the domains lfr serves straight into their
// mailboxes, the way every MTA treats its own domains: no relay cost, no
// round trip, and no dependence on DNS that may not be published yet. Only
// messages lfr built itself pass here, so they count as authenticated.
type localFirst struct {
	outbound.Transport
	e *Engine
}

// Relayed passes through whether the wrapped transport is a relay.
func (l localFirst) Relayed() bool {
	r, ok := l.Transport.(interface{ Relayed() bool })
	return ok && r.Relayed()
}

func (l localFirst) Send(ctx context.Context, from, rcpt string, raw []byte) error {
	domain := addr.DomainOf(rcpt)
	if _, err := l.e.Domains.Get(ctx, domain); err != nil {
		return l.Transport.Send(ctx, from, rcpt, raw)
	}
	ok, err := l.e.accept(ctx, rcpt)
	if err != nil {
		return &outbound.Failure{Temporary: true, Message: "local delivery: " + err.Error()}
	}
	if !ok {
		return &outbound.Failure{Code: 550, Enhanced: "5.1.1", Bounce: true, Message: "no such mailbox here"}
	}
	host := l.e.Hostname(ctx)
	p := msg.Parse(raw)
	fromDomain := addr.DomainOf(p.From.Email)
	auth := mailauth.Result{SPF: "pass", SPFDomain: addr.DomainOf(from), FromDomain: fromDomain, DMARC: "pass",
		DKIM: []mailauth.DKIMResult{{Domain: fromDomain, Result: "pass"}}}
	trace := fmt.Sprintf("Return-Path: <%s>\r\nAuthentication-Results: %s; local=pass (sent by this server)\r\nReceived: by %s (lfr) with LOCAL; %s\r\n",
		from, host, host, l.e.Opts.Now().Format(time.RFC1123Z))
	env := &inbound.Envelope{From: from, Rcpts: []string{strings.ToLower(rcpt)}, Parsed: p, Auth: auth,
		Raw: append([]byte(trace), raw...)}
	if err := l.e.deliver(ctx, env); err != nil {
		return &outbound.Failure{Temporary: true, Message: "local delivery: " + err.Error()}
	}
	return nil
}
