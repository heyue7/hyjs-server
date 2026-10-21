# 原生回收屋一键选择修复

沿原 Pages/RecycleBreakupSelectPage，替换原角色/装备/精灵三处 OnPetBreakupButtionOneKeyClick；保留原窗口、checkbox、确定、4槽及原分解协议。无新按钮或一键销毁入口。

SelectionControl 请求当前原服务 origin 的 POST /recycle/selection，先清原缓存，显示服务器验证通过的 GUID。等待期间临时禁确认和原列表行点击，原 checkbox 可取消；退出/切换/旧会话回包不更新现页。网络错误恢复可重试，不写财产。

`build.py --base-apk <qualified.apk> --output-directory <fresh-folder>` 使用原Unity引用编译helper及三方法注入，并逐方法验证其他raw CIL和547原引擎精确。与终端DEX合并后用tools/client_release/package_native.py签名打包。

仅本机QA：`qa_package.py --base-apk <signed.apk> --expected-sha <sha256> --output-directory <fresh-folder> --record <fresh.json>`，从包内实际读取原origin，将四处原HTTP origin和钱包JSON改为127.0.0.1:28935；原Java及其他压缩entry精确，签名保持。不安装、不提升current，禁止把QA包作为正式交付。

服务端的选取只读验证实际分解/返还资格；最终确认仍走原21021、21009、21517及其事务回执。绿色/蓝色/紫色角色排除达尔肯及实际使用占用，装备/精灵沿原自动选取品质与成长条件。验证记录在analysis/terminal-b40-20261021。
