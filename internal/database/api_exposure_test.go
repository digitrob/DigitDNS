package database

import "testing"

func TestGetAPIExposureListOutranksSingle(t *testing.T) {
	s := &ServerSettings{
		APIBind:     "127.0.0.1",
		APIBindList: []string{"100.64.0.0/10", "203.0.113.5"},
	}
	list, single := s.GetAPIExposure()
	if single != "" {
		t.Fatalf("single = %q, want empty when the list is authoritative", single)
	}
	if len(list) != 2 || list[0] != "100.64.0.0/10" || list[1] != "203.0.113.5" {
		t.Fatalf("list = %v, want the stored rules", list)
	}
}

func TestGetAPIExposureFallsBackToSingleWhenListEmpty(t *testing.T) {
	s := &ServerSettings{APIBind: "203.0.113.7", APIBindList: nil}
	list, single := s.GetAPIExposure()
	if list != nil {
		t.Fatalf("list = %v, want nil when the single rule is authoritative", list)
	}
	if single != "203.0.113.7" {
		t.Fatalf("single = %q, want the stored value", single)
	}
}

func TestGetAPIExposureNilReceiverIsEmpty(t *testing.T) {
	var s *ServerSettings
	if list, single := s.GetAPIExposure(); list != nil || single != "" {
		t.Fatalf("nil receiver = %v/%q, want empty", list, single)
	}
}

func TestGetAPIBindListReturnsACopy(t *testing.T) {
	s := &ServerSettings{APIBindList: []string{"100.64.0.0/10"}}
	got := s.GetAPIBindList()
	got[0] = "0.0.0.0"
	if s.APIBindList[0] != "100.64.0.0/10" {
		t.Fatal("GetAPIBindList returned a slice aliasing the stored field")
	}
}
