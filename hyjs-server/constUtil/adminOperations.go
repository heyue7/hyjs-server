package constUtil

import "errors"

const AdminPrincipalContextKey = "hyjs-admin-principal"
const AdminCatalogueCategories = "currency,character,equipment,accessory,pet,item,fragment,gem,cooking"

var ErrAdminConfigConflict = errors.New("配置已更新或已冻结，请重新读取；修改已冻结内容请复制为新活动期")
var ErrAdminInput = errors.New("后台参数或资源配置无效")

// AdminCommerceMysticKind 是原神秘商店业务类型。
const AdminCommerceMysticKind = "mystic"
