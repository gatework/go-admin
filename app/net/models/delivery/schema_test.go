package delivery

import (
	"sync"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm/schema"
)

func TestMySQLStorageFitsScriptAndOutputBounds(t *testing.T) {
	s, e := schema.Parse(&Target{}, &sync.Map{}, schema.NamingStrategy{})
	if e != nil {
		t.Fatal(e)
	}
	d := mysql.New(mysql.Config{})
	for _, name := range []string{"Script", "Output"} {
		typ := d.DataTypeOf(s.LookUpField(name))
		if typ != "mediumtext" && typ != "longtext" {
			t.Errorf("%s uses %s, too short for delivery limits", name, typ)
		}
	}
}
