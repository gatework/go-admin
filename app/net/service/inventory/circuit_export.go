package inventory

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"time"

	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
	"github.com/gatework/motor/common/csvsafe"
)

// Export provides the export operation within the caller's authorized network scope.
func (s *Circuit) Export(ctx context.Context, r *dto.CircuitPageReq, p *actions.DataPermission) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	var rows []inventory.Circuit
	if err := s.scoped(ctx, p).Scopes(circuitFilters(r)).Order("updated_at DESC, id DESC").Limit(10001).Find(&rows).Error; err != nil {
		return nil, inventory.PersistenceErr
	}
	if len(rows) > 10000 {
		return nil, inventory.CircuitExportLimitErr
	}
	buffer := bytes.NewBufferString("\ufeff")
	writer := csv.NewWriter(buffer)
	headers := []string{"ID", "线路编号", "名称", "运营商", "类型", "用途", "状态", "带宽 Mbps", "地区", "组织", "合同号", "开通日期", "到期日期", "付款方", "费用", "币种", "账期", "路由说明", "备注"}
	for _, side := range []string{"A", "Z"} {
		for _, label := range []string{"设备 ID", "端点名称", "地址", "端口", "IP", "联系人", "联系电话", "监控主机 ID", "监控主机", "入站监控项 ID", "入站监控项", "出站监控项 ID", "出站监控项"} {
			headers = append(headers, side+" 端"+label)
		}
	}
	headers = append(headers, "创建时间", "更新时间")
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, r := range rows {
		bandwidth := ""
		if r.BandwidthMbps != nil {
			bandwidth = strconv.Itoa(*r.BandwidthMbps)
		}
		cells := []string{strconv.Itoa(r.ID), r.CircuitNo, r.Name, r.Provider, r.Category, r.Purpose, r.Status, bandwidth, r.Region, r.Organization, r.ContractNo, r.InstalledOn, r.ExpiresOn, r.Payer, r.Fee, r.Currency, r.BillingCycle, r.RouteNotes, r.Notes}
		for _, e := range []inventory.CircuitEndpoint{r.AEndpoint, r.ZEndpoint} {
			device := ""
			if e.DeviceID != nil {
				device = strconv.Itoa(*e.DeviceID)
			}
			b := e.Monitoring
			cells = append(cells, device, e.Name, e.Address, e.Interface, e.IP, e.ContactName, e.ContactPhone, b.HostID, b.HostName, b.InItemID, b.InItemName, b.OutItemID, b.OutItemName)
		}
		cells = append(cells, r.CreatedAt.UTC().Format(time.RFC3339), r.UpdatedAt.UTC().Format(time.RFC3339))
		for i := range cells {
			cells[i] = csvsafe.Cell(cells[i])
		}
		if err := writer.Write(cells); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}
