package constUtil

import "errors"

// Live shop quote IDs keep native protobuf fields and stable stock identities.
const VIPShopQuotePrefix = "vipq:"
const UnionFightShopQuotePrefix = "ufq:"
const UnionFightShopTokenResource = "unionFightToken"
const LiveShopQuoteHashLength = 16

var ErrShopQuoteChanged = errors.New("商品报价已变化，请刷新商店")
