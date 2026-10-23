# 售罄阶梯报价的只读展示

`QuotedShopCost.Price`只增加一个受限分支：p4qt、dayLeftTimes=0、dayTotalTimes>0、canBuy=0，且请求ordinal精确等于total+1时返回服务端最后有效wire价。使用long比较避免整数溢出。该分支不访问下一档、不弹报价过期、不修改canBuy/dayLeftTimes。真实还有额度、非法ordinal/total或矛盾资格仍沿原校验。fixed p4q/s1q不变。

这修复的是原快捷使用每次成功回调SetItem时，售罄第20次wire50与原第21次80的误报。使用操作原CSItemUse从未改购物counter；服务端限购/重算/配置报价hash验证保持。不能用此分支购买售罄商品。

`build_quote_helper_overlay.py`固定base52 APK/SHA，编译HeroArt.cs、UFQuotedCost.cs、QuotedShopCost.cs、HeroCopyTable.g.cs全部源，不注入main；用dnlib比较旧新helper全部type/method/field签名与程序集identity，输出仅SoulFlowerHeroArt.dll的managed-only合格overlay。

`verify_quote_boundaries.py`直接提取实际Quote/Price方法源，使用原TableItemShopPrice的itemShopId_4行与最小UI doubles执行13项边界；连续库存使用部分只模拟原回调重绘，不声称原Unity/网络CSItemUse集成或原机通过。实际原引用C#3编译和旧helper严格API验证单独完成。
