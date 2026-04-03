/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"strings"
	"testing"
)

func TestEdge_TicketNumberOfUsesZeroRejected(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}
	zero := 0
	ticket := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			NumberOfUses:  &zero,
		},
	}

	_, err := v.ValidateCreate(context.Background(), ticket)
	if err == nil {
		t.Fatal("expected rejection for numberOfUses = 0")
	}
	if !strings.Contains(err.Error(), "numberOfUses") {
		t.Errorf("error should mention numberOfUses, got: %v", err)
	}
}

func TestEdge_TicketNumberOfUsesNegativeRejected(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}
	neg := -5
	ticket := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			NumberOfUses:  &neg,
		},
	}

	_, err := v.ValidateCreate(context.Background(), ticket)
	if err == nil {
		t.Fatal("expected rejection for negative numberOfUses")
	}
}

func TestEdge_TicketSpecChangeOnUpdateRejected(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}
	uses := 3

	old := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			TargetName:    "web-server",
			NumberOfUses:  &uses,
		},
	}

	// Change the target name — should be rejected because tickets are immutable.
	changed := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			TargetName:    "different-server",
			NumberOfUses:  &uses,
		},
	}

	_, err := v.ValidateUpdate(context.Background(), old, changed)
	if err == nil {
		t.Fatal("expected rejection when ticket spec changes on update")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Errorf("error should mention immutable, got: %v", err)
	}
}

func TestEdge_TicketSpecChangeConnectionRefRejected(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}

	old := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
		},
	}
	changed := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "other-conn",
			Username:      "admin",
		},
	}

	_, err := v.ValidateUpdate(context.Background(), old, changed)
	if err == nil {
		t.Fatal("expected rejection when connectionRef changes on update")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Errorf("error should mention immutable, got: %v", err)
	}
}

func TestEdge_TicketSpecChangeDescriptionRejected(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}

	old := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			Description:   "original",
		},
	}
	changed := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			Description:   "modified",
		},
	}

	_, err := v.ValidateUpdate(context.Background(), old, changed)
	if err == nil {
		t.Fatal("expected rejection when description changes on update")
	}
}

func TestEdge_TicketSpecNumberOfUsesAddedOnUpdateRejected(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}

	old := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
		},
	}
	uses := 5
	changed := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			NumberOfUses:  &uses,
		},
	}

	_, err := v.ValidateUpdate(context.Background(), old, changed)
	if err == nil {
		t.Fatal("expected rejection when numberOfUses is added on update")
	}
}

func TestEdge_TicketOnlyConnectionRefWarningReturned(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}
	ticket := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
		},
	}

	warnings, err := v.ValidateCreate(context.Background(), ticket)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning when neither username nor targetName is set")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "username") || strings.Contains(w, "targetName") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning to mention username or targetName, got: %v", warnings)
	}
}

func TestEdge_TicketDeleteAlwaysAllowed(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}

	// Even a ticket with no fields should be deletable.
	_, err := v.ValidateDelete(context.Background(), &WarpgateTicket{})
	if err != nil {
		t.Fatalf("expected delete to always succeed, got: %v", err)
	}

	// A fully populated ticket should also be deletable.
	uses := 10
	_, err = v.ValidateDelete(context.Background(), &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			TargetName:    "target",
			NumberOfUses:  &uses,
			Description:   "some ticket",
			Expiry:        "2026-12-31T23:59:59Z",
		},
	})
	if err != nil {
		t.Fatalf("expected delete to always succeed for populated ticket, got: %v", err)
	}
}

func TestEdge_TicketIdenticalSpecOnUpdateAccepted(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}
	uses := 3

	spec := WarpgateTicketSpec{
		ConnectionRef: "my-conn",
		Username:      "admin",
		TargetName:    "web-server",
		NumberOfUses:  &uses,
		Description:   "a ticket",
		Expiry:        "2026-12-31T23:59:59Z",
	}

	// Both use the same values but different pointer addresses for numberOfUses.
	usesCopy := 3
	old := &WarpgateTicket{Spec: spec}
	same := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			TargetName:    "web-server",
			NumberOfUses:  &usesCopy,
			Description:   "a ticket",
			Expiry:        "2026-12-31T23:59:59Z",
		},
	}

	_, err := v.ValidateUpdate(context.Background(), old, same)
	if err != nil {
		t.Fatalf("expected identical spec to pass update validation, got: %v", err)
	}
}
