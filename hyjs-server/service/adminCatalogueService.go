package service

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"hyjs-server/model"
)

// adminCatalogueMedia 只读取导出清单中的真实名称与原像素地址。
type adminCatalogueMedia struct {
	Sprites map[string]struct {
		IconURL    string `json:"iconUrl"`
		PreviewURL string `json:"previewUrl"`
	} `json:"sprites"`
	DisplayNames map[string]struct {
		Text string `json:"text"`
	} `json:"displayNames"`
	ResourceIcons map[string]string `json:"resourceIcons"`
	ActivityModes map[string]struct {
		PreviewURLs []string `json:"previewUrls"`
	} `json:"activityModes"`
}

// LoadAdminCatalogue 独立加载管理目录，不改变战斗Tables或它们已有摘要。
func (s *GameService) LoadAdminCatalogue(rootTables, mediaManifest string) error {
	tables := map[string][]map[string]string{}
	for _, name := range []string{"Text", "Hero", "Equip", "Treasure", "Pet", "Frag", "Item", "FishSkill", "MineSkill", "PetEquip", "UnionFightItem"} {
		rows, err := adminCatalogueReadTSV(filepath.Join(rootTables, "Table"+name+".tsv"))
		if err != nil {
			return fmt.Errorf("管理目录%s: %w", name, err)
		}
		tables[name] = rows
	}
	// 启动兼容已经补齐的角色/碎片覆盖同ID字段，并纳入六绿新碎片；只建立目录副本。
	for _, name := range []string{"Hero", "Frag"} {
		merged := map[string]map[string]string{}
		for _, row := range tables[name] {
			copyRow := map[string]string{}
			for key, value := range row {
				copyRow[key] = value
			}
			merged[row["id"]] = copyRow
		}
		for _, row := range s.Tables[name] {
			if row["id"] == "" {
				continue
			}
			copyRow := merged[row["id"]]
			if copyRow == nil {
				copyRow = map[string]string{}
				merged[row["id"]] = copyRow
			}
			for key, value := range row {
				copyRow[key] = value
			}
		}
		tables[name] = nil
		for _, row := range merged {
			tables[name] = append(tables[name], row)
		}
	}
	var media adminCatalogueMedia
	if mediaManifest != "" {
		raw, err := os.ReadFile(mediaManifest)
		if err != nil {
			return fmt.Errorf("管理目录原图清单: %w", err)
		}
		if err = json.Unmarshal(raw, &media); err != nil {
			return fmt.Errorf("管理目录原图清单: %w", err)
		}
	}
	s.AdminCatalogueData = adminCatalogueBuild(tables, media)
	s.AdminCatalogueData.SourceTables = tables
	// Presentation-only consumers reuse exported original pixels without modifying gameplay tables.
	s.AdminCatalogueData.SpriteURLs = map[string]string{}
	for key, sprite := range media.Sprites {
		if url := adminCatalogueSafeMediaURL(sprite.IconURL); url != "" {
			s.AdminCatalogueData.SpriteURLs[key] = url
		}
	}
	s.AdminCatalogueData.DisplayNames = map[string]string{}
	for key, label := range media.DisplayNames {
		if label.Text != "" {
			s.AdminCatalogueData.DisplayNames[key] = label.Text
		}
	}
	return nil
}

// adminCatalogueReadTSV 按原表五行元数据格式建立独立字段映射。
func adminCatalogueReadTSV(path string) ([]map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 5 {
		return nil, fmt.Errorf("缺少五行原表元数据")
	}
	out := []map[string]string{}
	for _, values := range rows[5:] {
		if len(values) == 0 || values[0] == "" {
			continue
		}
		row := map[string]string{}
		for i, value := range values {
			if i < len(rows[1]) {
				row[rows[1][i]] = value
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func adminCatalogueFirst(row map[string]string, keys ...string) string {
	for _, key := range keys {
		if row[key] != "" {
			return row[key]
		}
	}
	return ""
}
func adminCatalogueNumber(value string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return n
}
func adminCatalogueSafeMediaURL(value string) string {
	if strings.HasPrefix(value, "/admin/media/") && !strings.Contains(value, "..") && !strings.ContainsAny(value, "?#\r\n\x00") {
		return value
	}
	return ""
}

// adminCatalogueItemCategoryLabel 沿TableItem首行类别说明展示，未说明的值保留原编号。
func adminCatalogueItemCategoryLabel(value string) string {
	labels := map[string]string{"1": "使用后增加属性", "2": "使用后获得物品", "4": "养成消耗材料", "5": "角色选择箱", "6": "免战道具", "7": "装备精炼石", "8": "精灵经验", "9": "亲密值", "11": "特殊用途道具", "12": "客户端展示道具", "13": "道具合成", "14": "充值与免费钻石", "15": "生活技能书", "17": "公会战商店道具", "18": "公会战战场道具", "19": "觉醒融合材料", "20": "跨服竞猜卷轴", "21": "烟花", "22": "福袋", "23": "光盘随机箱", "24": "阵法经验道具", "25": "精灵装备觉醒", "26": "精灵装备升阶", "27": "角色技能专精", "28": "装备升星"}
	if label := labels[value]; label != "" {
		return label
	}
	return "道具类别" + value
}

// adminCatalogueQuality 将管理筛选统一为颜色；原表与游戏计算始终保留原值。
// 资质12/13/18/20/23对应绿蓝紫橙红；小品级沿白绿蓝紫橙红显示，高于红阶归入红色。
func adminCatalogueQuality(original int64) (int64, string, string) {
	quality := original
	if mapped, ok := map[int64]int64{12: 2, 13: 3, 18: 4, 20: 5, 23: 6}[original]; ok {
		quality = mapped
	} else if original == 7 {
		quality = 6
	}
	labels := []string{"未标注", "白色 · 普通", "绿色 · 优秀", "蓝色 · 稀有", "紫色 · 史诗", "橙色 · 传说", "红色 · 神话"}
	colors := []string{"gray", "white", "green", "blue", "purple", "orange", "red"}
	if quality < 0 || quality >= int64(len(labels)) {
		quality = 0
	}
	return quality, labels[quality], colors[quality]
}

// adminCatalogueBuild 将原目录与单独文本索引合并，缺失名称保持明确原ID。
func adminCatalogueBuild(tables map[string][]map[string]string, media adminCatalogueMedia) *model.AdminCatalogueData {
	texts := map[string]string{}
	for _, row := range tables["Text"] {
		if row["id"] != "" && row["text"] != "" {
			texts[row["id"]] = row["text"]
		}
	}
	for key, value := range media.DisplayNames {
		if texts[key] == "" {
			texts[key] = value.Text
		}
	}
	resolve := func(value string) string {
		if value == "" {
			return ""
		}
		if translated := texts[value]; translated != "" {
			return translated
		}
		for _, prefix := range []string{"name_", "intro_", "desc_", "description_"} {
			if strings.HasPrefix(value, prefix) {
				return ""
			}
		}
		return value
	}
	names := func(row map[string]string, id string) string {
		for _, value := range []string{row["name"], row["Name"], row["nameIndex"], "name_" + id} {
			if name := resolve(value); name != "" {
				return name
			}
		}
		return id
	}
	descriptions := func(row map[string]string) string { return resolve(adminCatalogueFirst(row, "desc", "intro", "Desc")) }
	categoryLabels := map[string]string{"currency": "货币", "character": "角色", "character-exp": "角色经验", "equipment": "装备", "accessory": "饰品", "pet": "精灵", "pet-equipment": "精灵装备", "item": "道具", "fragment": "碎片", "gem": "宝石", "cooking": "料理", "material": "生活材料"}
	typeLabels := map[string]string{}
	addType := func(item *model.AdminItemDTO, key, label string) {
		if key == "" {
			return
		}
		item.ApplicableTypes = append(item.ApplicableTypes, key)
		typeLabels[key] = label
	}
	byID := map[string]model.AdminItemDTO{}
	image := func(item *model.AdminItemDTO, row map[string]string) {
		item.Icon = media.ResourceIcons[item.ID]
		if item.Icon == "" {
			item.Icon = adminCatalogueFirst(row, "icon", "iconName")
		}
		sprite := media.Sprites[item.Icon]
		item.IconURL = adminCatalogueSafeMediaURL(sprite.IconURL)
		item.PreviewURL = adminCatalogueSafeMediaURL(sprite.PreviewURL)
		for _, key := range []string{"pictrue", "png", "picture"} {
			if big := media.Sprites[row[key]]; big.IconURL != "" {
				item.PreviewURL = adminCatalogueSafeMediaURL(big.IconURL)
				break
			}
		}
		if item.PreviewURL == "" {
			item.PreviewURL = item.IconURL
		}
	}
	put := func(item model.AdminItemDTO, row map[string]string) {
		if item.ID == "" {
			return
		}
		if _, ok := byID[item.ID]; ok {
			return
		}
		item.Name = names(row, item.ID)
		item.Description = descriptions(row)
		if item.ApplicableTypes == nil {
			item.ApplicableTypes = []string{}
		}
		image(&item, row)
		byID[item.ID] = item
	}
	for _, row := range tables["Hero"] {
		id := row["id"]
		if !strings.HasPrefix(id, "hero_") {
			continue
		}
		quality := adminCatalogueNumber(row["quality"])
		item := model.AdminItemDTO{ID: id, Category: "character", Quality: quality, QualityLabel: fmt.Sprintf("资质%d", quality)}
		if row["heroClass"] == "99" {
			item.Category = "character-exp"
		}
		if row["heroClass"] != "" {
			addType(&item, "hero.heroClass:"+row["heroClass"], "角色类型"+row["heroClass"])
		}
		put(item, row)
	}
	for _, row := range tables["Equip"] {
		quality := adminCatalogueNumber(row["quality"])
		item := model.AdminItemDTO{ID: row["id"], Category: "equipment", Quality: quality, QualityLabel: fmt.Sprintf("资质%d", quality)}
		if value := row["type"]; value != "" {
			label := texts["equip_type_"+value]
			if label == "" {
				label = "装备类型" + value
			}
			addType(&item, "equip.type:"+value, label)
		}
		put(item, row)
	}
	for _, row := range tables["Treasure"] {
		quality := adminCatalogueNumber(row["quality"])
		item := model.AdminItemDTO{ID: row["id"], Category: "accessory", Quality: quality, QualityLabel: fmt.Sprintf("资质%d", quality)}
		if value := row["type"]; value != "" {
			label := map[string]string{"1": "戒指", "0": "项链", "-1": "饰品经验"}[value]
			if label == "" {
				label = "饰品类型" + value
			}
			addType(&item, "treasure.type:"+value, label)
		}
		put(item, row)
	}
	for _, row := range tables["Pet"] {
		quality := adminCatalogueNumber(row["quality"])
		item := model.AdminItemDTO{ID: row["id"], Category: "pet", Quality: quality, QualityLabel: fmt.Sprintf("资质%d", quality)}
		if value := row["nature"]; value != "" {
			addType(&item, "pet.nature:"+value, "精灵向性"+value)
		}
		put(item, row)
	}
	for _, row := range tables["Item"] {
		quality := adminCatalogueNumber(row["star"])
		item := model.AdminItemDTO{ID: row["itemId"], Category: "item", Quality: quality}
		if quality > 0 {
			item.QualityLabel = fmt.Sprintf("%d星", quality)
		}
		if value := row["category"]; value != "" {
			addType(&item, "item.category:"+value, adminCatalogueItemCategoryLabel(value))
		}
		put(item, row)
	}
	for _, row := range tables["FishSkill"] {
		kind := row["Type"]
		if kind == "dock" {
			continue
		}
		quality := adminCatalogueNumber(row["Quality"])
		item := model.AdminItemDTO{ID: row["FishItemID"], Category: "material", Quality: quality}
		if kind == "dish" {
			item.Category = "cooking"
		}
		if quality > 0 {
			item.QualityLabel = fmt.Sprintf("品级%d", quality)
		}
		if kind != "" {
			label := map[string]string{"fish": "鱼类食材", "dish": "料理"}[kind]
			if label == "" {
				label = "烹饪类型" + kind
			}
			addType(&item, "fish.Type:"+kind, label)
		}
		put(item, row)
	}
	for _, row := range tables["MineSkill"] {
		kind := row["Type"]
		if kind == "mine" {
			continue
		}
		quality := adminCatalogueNumber(row["Quality"])
		item := model.AdminItemDTO{ID: row["MineItemID"], Category: "material", Quality: quality}
		if kind == "magicGem" {
			item.Category = "gem"
		}
		if quality > 0 {
			item.QualityLabel = fmt.Sprintf("品级%d", quality)
		}
		if kind != "" {
			label := map[string]string{"stone": "矿石", "gem": "宝石材料", "magicGem": "可镶嵌宝石", "magicStone": "魔石", "powder": "催化粉尘"}[kind]
			if label == "" {
				label = "采矿类型" + kind
			}
			addType(&item, "mine.Type:"+kind, label)
		}
		put(item, row)
	}
	for _, row := range tables["PetEquip"] {
		item := model.AdminItemDTO{ID: row["id"], Category: "pet-equipment"}
		if target, ok := byID[row["petId"]]; ok {
			item.Quality = target.Quality
			item.QualityLabel = target.QualityLabel
		}
		if value := row["position"]; value != "" {
			addType(&item, "petEquip.position:"+value, "精灵装备位置"+value)
		}
		put(item, row)
	}
	// 碎片名称、资质和适用类型沿真实合成目标继承，不把碎片ID当文本索引。
	for _, row := range tables["Frag"] {
		id := row["id"]
		item := model.AdminItemDTO{ID: id, Category: "fragment", Name: id, ApplicableTypes: []string{}}
		if target, ok := byID[row["target"]]; ok {
			item.Name = target.Name + "碎片"
			item.Quality = target.Quality
			item.QualityLabel = target.QualityLabel
			item.Icon = target.Icon
			item.IconURL = target.IconURL
			item.PreviewURL = target.PreviewURL
			item.ApplicableTypes = append(item.ApplicableTypes, target.ApplicableTypes...)
			addType(&item, "fragment.target:"+target.Category, categoryLabels[target.Category]+"碎片")
		}
		byID[id] = item
	}
	for id, name := range map[string]string{"coin": "金币", "diamond": "钻石", "energy": "精力", "stamina": "体力", "light": "滞留之光", "point": "攻略点数", "popularity": "声望", "score": "悬赏积分", "coinpet": "精灵币", "contribution": "公会贡献", "unionFightToken": "公会战代币"} {
		item := model.AdminItemDTO{ID: id, Name: name, Category: "currency", ApplicableTypes: []string{}}
		item.Icon = media.ResourceIcons[id]
		if item.Icon == "" {
			// 原TableItem的item_200/201分别是金币/钻石，图标沿其明确绑定。
			item.Icon = map[string]string{"coin": "item_icon_200", "diamond": "item_icon_201"}[id]
		}
		sprite := media.Sprites[item.Icon]
		item.IconURL = adminCatalogueSafeMediaURL(sprite.IconURL)
		item.PreviewURL = adminCatalogueSafeMediaURL(sprite.PreviewURL)
		if item.PreviewURL == "" {
			item.PreviewURL = item.IconURL
		}
		byID[id] = item
	}
	result := &model.AdminCatalogueData{ActivityPreviews: map[string]string{}, Items: []model.AdminItemDTO{}, Categories: []string{}, Facets: model.AdminCatalogueFacets{Categories: []model.AdminCatalogueOption{}, Qualities: []model.AdminCatalogueQualityOption{}, ApplicableTypes: []model.AdminCatalogueOption{}}}
	// 公会战专用道具含无背包映射的五种资源，名称和缩略图直接沿原表解析。
	for _, row := range tables["UnionFightItem"] {
		item := model.AdminItemDTO{ID: row["UFItemId"], Name: resolve(row["ItemName"]), Category: "union-fight", Icon: row["Icon"], QualityLabel: "原公会战道具", QualityColor: "gray"}
		sprite := media.Sprites[item.Icon]
		item.IconURL = adminCatalogueSafeMediaURL(sprite.IconURL)
		item.PreviewURL = adminCatalogueSafeMediaURL(sprite.PreviewURL)
		if item.PreviewURL == "" {
			item.PreviewURL = item.IconURL
		}
		result.UnionFightItems = append(result.UnionFightItems, item)
	}
	// 清单只导出原页面部件，优先使用活动专属部件并保留明确的预览边界。
	for mode, original := range media.ActivityModes {
		for _, url := range original.PreviewURLs {
			if strings.HasPrefix(url, "/admin/media/activity/") && adminCatalogueSafeMediaURL(url) != "" {
				result.ActivityPreviews[mode] = url
				break
			}
		}
		if result.ActivityPreviews[mode] == "" {
			for _, url := range original.PreviewURLs {
				if safe := adminCatalogueSafeMediaURL(url); safe != "" {
					result.ActivityPreviews[mode] = safe
					break
				}
			}
		}
	}
	categorySet := map[string]bool{}
	qualities := map[int64]bool{}
	for _, item := range byID {
		item.OriginalQuality = item.Quality
		item.Quality, item.QualityLabel, item.QualityColor = adminCatalogueQuality(item.OriginalQuality)
		sort.Strings(item.ApplicableTypes)
		result.Items = append(result.Items, item)
		categorySet[item.Category] = true
		qualities[item.Quality] = true
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].ID < result.Items[j].ID })
	for category := range categorySet {
		result.Categories = append(result.Categories, category)
	}
	sort.Strings(result.Categories)
	for _, category := range result.Categories {
		result.Facets.Categories = append(result.Facets.Categories, model.AdminCatalogueOption{Value: category, Label: categoryLabels[category]})
	}
	for value := range qualities {
		_, label, _ := adminCatalogueQuality(value)
		result.Facets.Qualities = append(result.Facets.Qualities, model.AdminCatalogueQualityOption{Value: value, Label: label})
	}
	sort.Slice(result.Facets.Qualities, func(i, j int) bool { return result.Facets.Qualities[i].Value < result.Facets.Qualities[j].Value })
	for key, label := range typeLabels {
		result.Facets.ApplicableTypes = append(result.Facets.ApplicableTypes, model.AdminCatalogueOption{Value: key, Label: label})
	}
	sort.Slice(result.Facets.ApplicableTypes, func(i, j int) bool {
		return result.Facets.ApplicableTypes[i].Value < result.Facets.ApplicableTypes[j].Value
	})
	return result
}

// AdminCatalogueFiltered 只在启动索引上筛选分页，不重扫TableText或触碰玩家存档。
func (s *GameService) AdminCatalogueFiltered(req model.AdminItemsRequest) model.AdminItemsResponse {
	data := s.AdminCatalogueData
	if data == nil {
		data = adminCatalogueBuild(s.Tables, adminCatalogueMedia{})
	}
	result := model.AdminItemsResponse{Items: []model.AdminItemDTO{}, Categories: append([]string{}, data.Categories...), Facets: data.Facets}
	q := strings.ToLower(req.Q)
	ids := map[string]bool{}
	for _, id := range req.IDs {
		ids[id] = true
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	size := req.PageSize
	if size < 1 {
		size = 25
	}
	items := data.Items
	if len(req.PreferredIDs) > 0 {
		items = append([]model.AdminItemDTO{}, items...)
		order := map[string]int{}
		for i, id := range req.PreferredIDs {
			if _, ok := order[id]; !ok {
				order[id] = i + 1
			}
		}
		sort.SliceStable(items, func(i, j int) bool {
			a, b := order[items[i].ID], order[items[j].ID]
			if a == 0 {
				a = len(order) + 1
			}
			if b == 0 {
				b = len(order) + 1
			}
			return a < b
		})
	}
	for _, item := range items {
		if req.Category != "" && item.Category != req.Category {
			continue
		}
		if req.Quality != nil && item.Quality != *req.Quality {
			continue
		}
		if len(ids) > 0 && !ids[item.ID] {
			continue
		}
		if req.ApplicableType != "" {
			found := false
			for _, key := range item.ApplicableTypes {
				if key == req.ApplicableType {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if q != "" && !strings.Contains(strings.ToLower(item.ID+" "+item.Name+" "+item.Description), q) {
			continue
		}
		result.Total++
		if result.Total <= (page-1)*size || int64(len(result.Items)) >= size {
			continue
		}
		item.ApplicableTypes = append([]string{}, item.ApplicableTypes...)
		result.Items = append(result.Items, item)
	}
	return result
}
