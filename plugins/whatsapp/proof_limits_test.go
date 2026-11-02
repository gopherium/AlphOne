// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"net/http"
	"testing"
)

func TestADeliveryStoresNoMoreItemsThanTheSettingAllows(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_DELIVERY_ITEMS_MAX": "2"})
	connectNumber(t, p, seedTenant(t, pool), numberWithSecret("5550001", "acme-secret"))
	body := deliveryOf(
		messageChange("5550001", "wamid.first"), messageChange("5550001", "wamid.second"),
		messageChange("5550001", "wamid.third"),
	)

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d so Meta stops retrying", recorder.Code, http.StatusOK)
	}
	if stored := countRows(t, pool, "plugin_whatsapp.messages"); stored != 2 {
		t.Errorf("messages = %d, want the setting's 2", stored)
	}
}

func TestADeliveryNamingMoreOwnSecretsThanTheSettingAllowsStoresNothing(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_DELIVERY_NUMBERS_MAX": "1"})
	acme := seedTenant(t, pool)
	connectNumber(t, p, acme, numberWithSecret("5550001", "acme-secret"))
	connectNumber(t, p, acme, numberWithSecret("5550002", "acme-secret"))
	body := deliveryOf(messageChange("5550001", "wamid.first"), messageChange("5550002", "wamid.second"))

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if held := heldConversations(t, pool, acme); held != 0 {
		t.Errorf("conversations = %d, want nothing stored over the setting", held)
	}
}

func TestStatusUpdatesCountTowardsTheItemsOfADelivery(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_DELIVERY_ITEMS_MAX": "2"})
	acme := seedTenant(t, pool)
	connectNumber(t, p, acme, numberWithSecret("5550001", "acme-secret"))
	seedOutboundRow(t, pool, "wamid.out.kept", acme)
	seedOutboundRow(t, pool, "wamid.out.over", acme)
	body := deliveryOf(
		messageChange("5550001", "wamid.first"), statusChange("5550001", "wamid.out.kept"),
		statusChange("5550001", "wamid.out.over"),
	)

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	wantStatus(t, pool, "wamid.out.kept", "delivered")
	wantNoStatus(t, pool, "wamid.out.over")
}

func TestUnprovenItemsTakeNoRoomInADelivery(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_DELIVERY_ITEMS_MAX": "1"})
	acme, globex := seedTenant(t, pool), seedGlobex(t, pool)
	connectNumber(t, p, acme, numberWithSecret("5550001", "acme-secret"))
	connectNumber(t, p, globex, numberWithSecret("5550002", "globex-secret"))
	body := deliveryOf(messageChange("5550002", "wamid.forged"), messageChange("5550001", "wamid.own"))

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := heldConversations(t, pool, acme); held != 1 {
		t.Errorf("conversations of the proven number = %d, want 1", held)
	}
}
