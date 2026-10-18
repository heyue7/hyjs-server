package constUtil

const (
	P13GuildRoomTarget      = "town_union"
	P13GuildRoomScopePrefix = "p13-union:"
	P13GuildRoomEntryNPC    = "npc_304"
	P13GuildRoomEntrySource = "具名本地领地入口策略：原请求零点或缺点使用原TableNPC npc_304公会大厅bornX/bornY/bornZ锚点；原服务固定出生点尚缺来源，不造坐标或改变有效重登位置。"
	P13GuildRoomSource      = "原UnionInfoTab.OnGotoUnionScene town_union/UIUnionRunMap，RoomUnion类型8/服务9，RoomNetUnion CSCurrentPosition与SCUnionBuilding，RoomNetBase.SCRefreshScene驱动CSLoginScene并按town_main/town_union切房。领地按真实guildId隔离，不创建未知建筑，未开启flag均0。"
)
