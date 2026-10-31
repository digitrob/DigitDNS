package service

import "testing"

// TestClientQueriesAreCountedAndFlushed: the detail screen's Queries figure comes
// from TotalQueries, which nothing used to write. The service counts lookups in
// memory and folds them into every read, then persists them with the usage flush.
func TestClientQueriesAreCountedAndFlushed(t *testing.T) {
	svc, db, cleanup := setupClientFixture(t)
	defer cleanup()

	client, err := svc.CreateClient("Query Counter", 30, "")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	for i := 0; i < 3; i++ {
		svc.RecordClientQuery(client.ID)
	}

	got, err := svc.GetClient(client.ID)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got.TotalQueries != 3 {
		t.Fatalf("GetClient total queries = %d, want 3", got.TotalQueries)
	}

	list, err := svc.ListClients()
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(list) != 1 || list[0].TotalQueries != 3 {
		t.Fatalf("ListClients = %+v, want one client with 3 queries", list)
	}

	if err := svc.FlushTraffic(); err != nil {
		t.Fatalf("FlushTraffic: %v", err)
	}
	stored, err := db.GetClient(client.ID)
	if err != nil {
		t.Fatalf("stored GetClient: %v", err)
	}
	if stored.TotalQueries != 3 {
		t.Fatalf("stored total queries = %d, want 3", stored.TotalQueries)
	}

	// The pending counter was drained, so a read must not add the same queries
	// again on top of the persisted total.
	got, err = svc.GetClient(client.ID)
	if err != nil {
		t.Fatalf("GetClient after flush: %v", err)
	}
	if got.TotalQueries != 3 {
		t.Fatalf("total queries after flush = %d, want 3 (not double counted)", got.TotalQueries)
	}
}
