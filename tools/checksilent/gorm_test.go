package main

import "testing"

func TestDiscardedGORMResultGuard(t *testing.T) {
	root := fixture(t, map[string]string{
		"app/shop/service/shop.go": `package service
import core "github.com/gatework/motor-core/sdk/service"
type Shop struct{core.Service}
func (e *Shop) Broken() { e.DB.First(new(int), 1); e.DB.Where("id = ?", 2).Save(new(int)) }
func (e *Shop) Checked() error { if err:=e.DB.First(new(int),1).Error;err!=nil{return err}; result:=e.DB.Save(new(int));return result.Error }
func (e *Shop) Builder() { e.DB.WithContext(nil) }
func (e *Shop) Shadow() { { e:=new(Unrelated); e.DB.First() } }
type Unrelated struct{DB Other}
type Other struct{}
func (Other) First() {}
func (e *Unrelated) Fine() { e.DB.First() }
`,
		"app/shop/service/shop_test.go": `package service
func example(e *Shop){e.DB.First(new(int),1)}
`,
	})
	findings := only(t, check(t, root, options{}), "gorm-discarded-result")
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want two discarded database results: %v", len(findings), findings)
	}
}
