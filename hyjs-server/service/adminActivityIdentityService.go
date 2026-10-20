package service

import (
	"strings"

	"hyjs-server/model"
)

// adminActivityCanonicalForm 将客户端固定标题与后台备注分开，历史快照仅在投影时整理。
// 名称依据原 UIActivity.InitTabs 与 TableText，王者招募沿用用户指定的统一名称。
func adminActivityCanonicalForm(form model.AdminActivityForm) model.AdminActivityForm {
	name := ""
	if form.Mode == "opening-14" {
		name = "开服狂欢"
	} else if definition, ok := adminActivityDefinitionFor(form.Mode); ok {
		name = definition.label
	}
	if name == "" || form.Mode == "shop-window" {
		return form
	}
	if form.Alias == "" && form.Name != "" && form.Name != name {
		form.Alias = form.Name
	}
	form.Name = name
	form.Alias = strings.TrimSpace(form.Alias)
	return form
}
