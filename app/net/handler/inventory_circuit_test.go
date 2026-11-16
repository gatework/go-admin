package handler

import (
	"fmt"
	"strings"
	"testing"

	models "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/inventory/provider/zabbix"
)

func TestCircuitAPIRejectsAuditMutationReturnsContractAndConflicts(t *testing.T) {
	t.Setenv("ZABBIX_API_URL", "")
	t.Setenv("ZABBIX_API_TOKEN", "")
	engine, db := apiTestEngine(t)
	if err := db.AutoMigrate(&models.Circuit{}); err != nil {
		t.Fatal(err)
	}
	a := Circuit{Zabbix: zabbix.FromEnvironment()}
	engine.GET("/circuits", a.List)
	engine.GET("/circuits/options", a.Options)
	engine.GET("/circuits/export", a.Export)
	engine.GET("/circuits/:id", a.Get)
	engine.GET("/circuits/:id/traffic", a.Traffic)
	engine.POST("/circuits", a.Create)
	engine.PUT("/circuits/:id", a.Update)
	engine.DELETE("/circuits/:id", a.Delete)
	for _, body := range []string{`{"circuitNo":"C-1","name":"one","provider":"carrier","createBy":8}`, `{"circuitNo":"C-1","name":"one","provider":"carrier","id":1}`, `{} {}`, `{"circuitNo":"C-1","name":"one","provider":"carrier","version":1}`} {
		_, result := serve(t, engine, "POST", "/circuits", body)
		if result["code"] != float64(400) {
			t.Fatalf("invalid create accepted: %+v", result)
		}
	}
	response, result := serve(t, engine, "POST", "/circuits", `{"circuitNo":"C-1","name":"one","provider":"carrier"}`)
	if result["code"] != float64(200) {
		t.Fatal(result)
	}
	data := result["data"].(map[string]any)
	if data["version"] != float64(1) || data["aEndpoint"] == nil || data["zEndpoint"] == nil || data["fee"] != "" || strings.Contains(response.Body.String(), "createBy") {
		t.Fatalf("wrong circuit contract: %s", response.Body.String())
	}
	id := int(data["id"].(float64))
	path := fmt.Sprintf("/circuits/%d", id)
	_, result = serve(t, engine, "PUT", path, `{"circuitNo":"C-1","name":"updated","provider":"carrier","version":1}`)
	if result["code"] != float64(200) {
		t.Fatal(result)
	}
	_, result = serve(t, engine, "DELETE", path, `{"version":1}`)
	if result["code"] != float64(409) {
		t.Fatalf("stale delete accepted: %+v", result)
	}
	_, result = serve(t, engine, "GET", path+"/traffic?period=1h", "")
	data = result["data"].(map[string]any)
	if data["configured"] != false || len(data["series"].([]any)) != 0 {
		t.Fatalf("missing config: %+v", result)
	}
	_, result = serve(t, engine, "GET", "/circuits?pageSize=201", "")
	if result["code"] != float64(400) {
		t.Fatal("unbounded page")
	}
	_, result = serve(t, engine, "GET", "/circuits", "")
	data = result["data"].(map[string]any)
	if data["count"] != float64(1) || len(data["list"].([]any)) != 1 {
		t.Fatalf("wrong page result: %+v", result)
	}
	response, _ = serve(t, engine, "GET", "/circuits/export", "")
	if !strings.Contains(response.Header().Get("Content-Disposition"), "attachment") || !strings.HasPrefix(response.Body.String(), "\ufeff") {
		t.Fatal("CSV contract")
	}
	_, result = serve(t, engine, "DELETE", path, `{"version":2}`)
	if result["code"] != float64(200) || result["data"] != nil {
		t.Fatalf("delete shape: %+v", result)
	}
}

func TestCircuitAPIDuplicateNumberIsRetryableValidationAndCASRemainsConflict(t *testing.T) {
	t.Setenv("ZABBIX_API_URL", "")
	t.Setenv("ZABBIX_API_TOKEN", "")
	engine, db := apiTestEngine(t)
	if err := db.AutoMigrate(&models.Circuit{}); err != nil {
		t.Fatal(err)
	}
	a := Circuit{Zabbix: zabbix.FromEnvironment()}
	engine.POST("/circuits", a.Create)
	engine.PUT("/circuits/:id", a.Update)
	engine.DELETE("/circuits/:id", a.Delete)
	_, result := serve(t, engine, "POST", "/circuits", `{"circuitNo":"C-1","name":"reserved","provider":"carrier"}`)
	if result["code"] != float64(200) {
		t.Fatal(result)
	}
	firstID := int(result["data"].(map[string]any)["id"].(float64))
	_, result = serve(t, engine, "POST", "/circuits", `{"circuitNo":" c-1 ","name":"duplicate","provider":"carrier"}`)
	if result["code"] != float64(400) || result["msg"] != "inventory.circuit_duplicate_number" {
		t.Errorf("duplicate create must be editable validation, not stale version: %+v", result)
	}
	_, result = serve(t, engine, "POST", "/circuits", `{"circuitNo":"C-2","name":"editable","provider":"carrier"}`)
	if result["code"] != float64(200) {
		t.Fatal(result)
	}
	secondID := int(result["data"].(map[string]any)["id"].(float64))
	path := fmt.Sprintf("/circuits/%d", secondID)
	_, result = serve(t, engine, "PUT", path, `{"circuitNo":"C-1","name":"must not persist","provider":"carrier","version":1}`)
	if result["code"] != float64(400) || result["msg"] != "inventory.circuit_duplicate_number" {
		t.Errorf("duplicate update must be editable validation: %+v", result)
	}
	var unchanged models.Circuit
	if err := db.First(&unchanged, secondID).Error; err != nil || unchanged.Version != 1 || unchanged.CircuitNo != "C-2" || unchanged.Name != "editable" {
		t.Fatalf("duplicate update consumed the version or wrote fields: %+v %v", unchanged, err)
	}
	_, result = serve(t, engine, "PUT", path, `{"circuitNo":"C-3","name":"corrected","provider":"carrier","version":1}`)
	if result["code"] != float64(200) || result["data"].(map[string]any)["version"] != float64(2) {
		t.Fatalf("correcting the number cannot retry at original version: %+v", result)
	}
	_, result = serve(t, engine, "PUT", path, `{"circuitNo":"C-3","name":"stale","provider":"carrier","version":1}`)
	if result["code"] != float64(409) {
		t.Errorf("stale CAS lost its conflict code: %+v", result)
	}
	_, result = serve(t, engine, "DELETE", fmt.Sprintf("/circuits/%d", firstID), `{"version":1}`)
	if result["code"] != float64(200) {
		t.Fatal(result)
	}
	_, result = serve(t, engine, "POST", "/circuits", `{"circuitNo":"C-1","name":"reuse deleted","provider":"carrier"}`)
	if result["code"] != float64(400) || result["msg"] != "inventory.circuit_duplicate_number" {
		t.Errorf("deleted number was reused or treated as CAS: %+v", result)
	}
	_, result = serve(t, engine, "PUT", path, `{"circuitNo":"C-1","name":"reuse deleted","provider":"carrier","version":2}`)
	if result["code"] != float64(400) || result["msg"] != "inventory.circuit_duplicate_number" {
		t.Errorf("update reused deleted number or was treated as CAS: %+v", result)
	}
}
