package dashboard

import "testing"

func TestResReqGapResult1971(t *testing.T) {
	r := ResReqGapResult1971{Summary: ResReqGapSummary1971{TotalContainers: 100, WithoutRequests: 20, WithoutLimits: 30, WithoutBoth: 10}}
	if r.Summary.WithoutBoth != 10 {
		t.Errorf("expected 10")
	}
}
func TestResReqGapEntry1971(t *testing.T) {
	e := ResReqGapEntry1971{Pod: "app-1", Namespace: "prod", Container: "web", HasRequest: false, HasLimit: false}
	if e.HasRequest {
		t.Errorf("expected false")
	}
}
func TestContainerPortResult1971(t *testing.T) {
	r := ContainerPortResult1971{Summary: ContainerPortSummary1971{TotalContainers: 50, WithPorts: 40, TotalPorts: 80, HostPorts: 5}}
	if r.Summary.HostPorts != 5 {
		t.Errorf("expected 5")
	}
}
func TestContainerPortEntry1971(t *testing.T) {
	e := ContainerPortEntry1971{Pod: "app", Namespace: "prod", Container: "web", Port: 8080, Name: "http", Protocol: "TCP", HasHostPort: false}
	if e.Port != 8080 {
		t.Errorf("expected 8080")
	}
}
func TestContainerPortDup1971(t *testing.T) {
	e := ContainerPortDup1971{Port: 30080, Containers: []string{"default/app/web", "prod/api/srv"}}
	if len(e.Containers) != 2 {
		t.Errorf("expected 2")
	}
}
func TestTermMsgResult1971(t *testing.T) {
	r := TermMsgResult1971{Summary: TermMsgSummary1971{TotalContainers: 60, WithTermMsgPath: 10, CustomPolicy: 5}}
	if r.Summary.CustomPolicy != 5 {
		t.Errorf("expected 5")
	}
}
func TestTermMsgEntry1971(t *testing.T) {
	e := TermMsgEntry1971{Pod: "app", Namespace: "prod", Container: "web", Issue: "sensitive path"}
	if e.Issue == "" {
		t.Errorf("expected non-empty")
	}
}
func TestResReqGapSummary1971(t *testing.T) {
	s := ResReqGapSummary1971{WithRequests: 80, WithLimits: 70}
	if s.WithRequests != 80 {
		t.Errorf("expected 80")
	}
}
