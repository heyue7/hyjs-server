# 大世界宠物跟随动作

原TablePet78个ID映射18套模型。幽梦兽五属性ID006/021/036/051/066共用pet_028；其低模原Animation没有clip，队伍高模有原stand/run。构建器仅补该模型的两个同源原clip、Animation引用与AssetBundle预加载索引，并更新smallassetmanifest对应行。其余原对象、模型/贴图/网格及全部托管方法保持。

`build.py --base-apk <固定1.0.49> --output-directory <全新build子目录>` 输出manifest及两项资源；再通过既有客户端更新构建和 `package.py --pet-follow-overlay` 签名。逐78行审查本地及外部clip引用、原骨骼有效绑定；原低模裁掉的末端辅助节点另列记录，不修改其他模型。

验证需原机观察真实播放，静态资源通过不等于运行通过。测试观察DLL仅置隔离QA APK，正式包不含观察代码。
