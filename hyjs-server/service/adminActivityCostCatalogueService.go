package service

import "hyjs-server/model"

// Filter through the real original parser before counting, facets or paging.
// The display catalogue alone includes objects that cannot be consumed as cost.
func (s *GameService) adminActivityCostCatalogue(preferred ...string) *model.AdminCatalogueData {
	all := s.AdminCatalogueFiltered(model.AdminItemsRequest{Page: 1, PageSize: 1 << 60})
	data := &model.AdminCatalogueData{Items: []model.AdminItemDTO{}, Categories: []string{}, Facets: model.AdminCatalogueFacets{Categories: []model.AdminCatalogueOption{}, Qualities: []model.AdminCatalogueQualityOption{}, ApplicableTypes: []model.AdminCatalogueOption{}}}
	allowUnionToken := false
	for _, id := range preferred {
		allowUnionToken = allowUnionToken || id == "unionFightToken"
	}
	categories := map[string]bool{}
	qualities := map[int64]bool{}
	types := map[string]bool{}
	for _, currency := range []bool{true, false} {
		for _, item := range all.Items {
			if (item.Category == "currency") != currency {
				continue
			}
			if _, err := s.p4ParseCost(item.ID + ",1"); err != nil && !(allowUnionToken && item.ID == "unionFightToken") {
				continue
			}
			data.Items = append(data.Items, item)
			categories[item.Category] = true
			qualities[item.Quality] = true
			for _, key := range item.ApplicableTypes {
				types[key] = true
			}
		}
	}
	for _, option := range all.Facets.Categories {
		if categories[option.Value] {
			data.Categories = append(data.Categories, option.Value)
			data.Facets.Categories = append(data.Facets.Categories, option)
		}
	}
	for _, option := range all.Facets.Qualities {
		if qualities[option.Value] {
			data.Facets.Qualities = append(data.Facets.Qualities, option)
		}
	}
	for _, option := range all.Facets.ApplicableTypes {
		if types[option.Value] {
			data.Facets.ApplicableTypes = append(data.Facets.ApplicableTypes, option)
		}
	}
	return data
}
