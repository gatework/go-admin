package wireless

import (
	"strings"

	"gorm.io/gorm"

	wireless "github.com/gatework/go-wireless"
	coredto "github.com/gatework/motor-core/sdk/contract/dto"
	models "github.com/gatework/motor/app/net/models/wireless"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
	"github.com/gatework/motor/common/database"
)

// The fact keys each resource list filter reads. A controller names the same
// value differently by platform and kind, so a filter matches any of them.
var (
	ipKeys     = []string{"ip_address", "ip", "client_ip", "client_ipv6", "management_ip"}
	macKeys    = []string{"mac_address", "client_mac", "bssid"}
	apKeys     = []string{"ap_name", "name", "ap_id"}
	ssidKeys   = []string{"ssid"}
	vlanKeys   = []string{"vlan", "vlan_id"}
	bandKeys   = []string{"band"}
	statusKeys = []string{"status", "operational_status"}
)

// searchColumns are the columns searchOf fills, for updates that select them.
var searchColumns = []string{"search_ip", "search_mac", "search_ap", "search_ssid", "search_vlan", "search_band", "search_status", "search_ap_name", "search_name", "search_text"}

// searchOf derives a resource's search columns from its facts.
func searchOf(facts wireless.Record) models.ResourceSearch {
	values := func(keys []string) string {
		out := make([]string, 0, len(keys))
		for _, k := range keys {
			if v := text(facts[k]); v != "" {
				out = append(out, strings.ToLower(v))
			}
		}
		return strings.Join(out, "\n")
	}
	return models.ResourceSearch{
		IP: values(ipKeys), MAC: values(macKeys), AP: values(apKeys), SSID: values(ssidKeys),
		VLAN: values(vlanKeys), Band: values(bandKeys), Status: values(statusKeys),
		APName: text(facts["ap_name"]), Name: text(facts["name"]),
		Text: strings.ToLower(text(facts)),
	}
}

// resourceFilter narrows resources by a list request: each field matches,
// case-insensitively, any fact it covers; the keyword also matches the
// management fields. ExactAP additionally requires the AP or resource name to
// equal AP.
func resourceFilter(r *dto.PageReq) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		like := " LIKE ? " + database.LikeEscape
		pattern := func(q string) string { return database.LikeContains(strings.ToLower(q)) }
		for column, q := range map[string]string{"search_ip": r.IP, "search_mac": r.MAC, "search_ap": r.AP, "search_ssid": r.SSID, "search_vlan": r.VLAN, "search_band": r.Band, "search_status": r.Status} {
			if q != "" {
				db = db.Where(column+like, pattern(q))
			}
		}
		if r.ExactAP && r.AP != "" {
			db = db.Where("(search_ap_name = ? OR search_name = ?)", r.AP, r.AP)
		}
		if r.Keyword != "" {
			k := pattern(r.Keyword)
			db = db.Where("(search_text"+like+" OR LOWER(local_alias)"+like+" OR LOWER(installation_location)"+like+" OR LOWER(owner)"+like+")", k, k, k, k)
		}
		return db
	}
}

// resourcePage counts the filtered resources and loads the requested page,
// by ID in the requested direction.
func resourcePage(q *gorm.DB, r *dto.PageReq) ([]models.Resource, int64, error) {
	q = q.Scopes(resourceFilter(r))
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return nil, 0, err
	}
	order := "id"
	if r.Direction == "desc" {
		order = "id DESC"
	}
	rows := []models.Resource{}
	err := q.Order(order).Scopes(coredto.Paginate(r.PageSize, r.PageIndex)).Find(&rows).Error
	return rows, n, err
}
