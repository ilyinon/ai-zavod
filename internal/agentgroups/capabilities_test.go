package agentgroups

import (
	"reflect"
	"testing"
)

func TestNormalizeDefaultSkills(t *testing.T) {
	got := NormalizeDefaultSkills([]string{" pony-tail ", "$security", "Security", "", "$CTF"})
	want := []string{"pony-tail", "security", "ctf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeDefaultSkills() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCapabilitiesFillsRoleDefaultSkills(t *testing.T) {
	tests := []struct {
		name string
		role string
		want []string
	}{
		{name: "developer", role: "developer", want: []string{"pony-tail", "dev"}},
		{name: "research", role: "researcher", want: []string{"pony-tail", "research"}},
		{name: "security", role: "security", want: []string{"pony-tail", "security"}},
		{name: "ctf", role: "ctf_pwn", want: []string{"pony-tail", "ctf"}},
		{name: "manager", role: "manager", want: []string{"pony-tail"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeCapabilities(Profile{RoleKey: tt.role}).DefaultSkills
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DefaultSkills = %#v, want %#v", got, tt.want)
			}
		})
	}
}
