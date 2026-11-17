package main

import "testing"

func TestSharedHandlerStateIsReported(t *testing.T) {
	root := fixture(t, map[string]string{
		"app/x/handler/h.go": `package handler
type API struct{ DB any }
type Firewall struct{ API }
func (e *Firewall) Shared(db any) { e.API = API{DB: db} }
func (e Firewall) Copied(db any) { e.API = API{DB: db} }
func prepare(e *Firewall, db any) { e.API = API{DB: db} }
`,
		"app/x/handler/batch.go": `package handler
type batch struct{ saved []string }
func (b *batch) persist(name string) { b.saved = append(b.saved, name) }
`,
		"app/x/service/s.go": `package service
type S struct{ n int }
func (s *S) Count() { s.n = 2 }
`,
	})
	if findings := only(t, check(t, root, options{}), checkSharedHandlerState); len(findings) != 1 {
		t.Fatalf("got %v, want only the pointer-receiver handler method", findings)
	}
}
