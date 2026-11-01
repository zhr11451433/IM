# IM 接口约束

> 本文件冻结对外入口。已列出的路径、方法、鉴权头和内部 Actor 方法名不得擅自新增、删除或改名。
> 路径来自 `F:\github_IM\im-server\services\apigateway\routers\router.go` 和 `services\navigator\routers\router.go`。内部方法名来自对应 handler 的实际调用。
> 各接口的 JSON 字段不在本文一次性展开。做到 `doc/task.md` 里的对应任务时，对照原 handler 和请求模型把字段补进该接口小节。补的是字段，不是改路径。

## 1. 服务端 API 通用约定

- Base URL：`http://127.0.0.1:9001`
- 前缀：`/apigateway`
- 整组路由使用签名中间件。
- 响应头带回 `request-id`，值为服务端生成的 `api_` 加短 ID。

请求头：

| 头 | 缺失时的错误 |
| --- | --- |
| `appkey` | `10001` appkey is required |
| `nonce` | `10002` nonce is required |
| `timestamp` | `10003` timestamp is required |
| `signature` | `10004` signature is required |

签名字符串是 `appSecret + nonce + timestamp`，再做 SHA1。摘要的大小写和编码以参考仓库 `commons/tools.SHA1` 的返回值为准，实现签名任务时核对，不在本文猜测。

其他已确认错误码：

| code | HTTP | 含义 |
| --- | --- | --- |
| 10005 | 500 | 应用不存在 |
| 10006 | 403 | 签名失败 |
| 10007 | 400 | 请求体非法 |
| 10008 | 500 | 内部调用超时 |
| 10009 | 500 | 内部调用失败 |
| 10010 | 400 | 缺少必填参数 |
| 10011 | 400 | 参数非法 |

业务错误码在实现对应模块时从参考仓库 `commons/errs` 按使用到的子集补入，不自造数字。

## 2. 服务端 HTTP 清单

共 77 条。`内部方法` 是原 handler 实际调用的 Actor 方法。私聊、系统消息、群消息和历史导入会先进入 `upstream`，再按真实方法名处理。

### 用户

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| POST | `/apigateway/users/register` | 注册用户 | `reg_user` |
| POST | `/apigateway/users/update` | 更新用户资料 | `upd_user_info` |
| POST | `/apigateway/users/settings/set` | 设置用户配置 | `set_user_settings` |
| GET | `/apigateway/users/settings/get` | 读取用户配置 | `get_user_settings` |
| GET | `/apigateway/users/info` | 查询用户信息 | `qry_user_info` |
| POST | `/apigateway/users/kick` | 踢下线 | `kick_user` |
| POST | `/apigateway/users/onlinestatus/query` | 查询在线状态 | `qry_online_status` |
| POST | `/apigateway/users/banusers/ban` | 封禁 | `ban_users` |
| POST | `/apigateway/users/banusers/unban` | 解封 | `ban_users` |
| GET | `/apigateway/users/banusers/query` | 查询封禁 | `qry_ban_users` |
| POST | `/apigateway/users/blockusers/block` | 拉黑 | `block_users` |
| POST | `/apigateway/users/blockusers/unblock` | 取消拉黑 | `block_users` |
| GET | `/apigateway/users/blockusers/query` | 查询拉黑 | `qry_block_users` |

### 机器人

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| POST | `/apigateway/bots/register` | 注册机器人 | `add_bot` |
| POST | `/apigateway/bots/update` | 更新机器人 | `upd_user_info` |

### 消息

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| POST | `/apigateway/messages/private/send` | 发私聊 | `upstream`，真实方法 `p_msg` |
| POST | `/apigateway/messages/system/send` | 发系统消息 | `upstream`，真实方法 `s_msg` |
| POST | `/apigateway/messages/group/send` | 发群消息 | `upstream`，真实方法 `g_msg` |
| POST | `/apigateway/messages/groupcast/send` | 按群组播 | `gc_msg` |
| POST | `/apigateway/messages/broadcast/send` | 广播 | `bc_msg` |
| POST | `/apigateway/messages/markread` | 标记已读 | `mark_read`，目标 ID 为会话 ID |
| POST | `/apigateway/messages/private/stream/send` | 私聊流式消息 | `send_stream_msg` |

`markread` 的处理函数在 `services/apigateway/apis/hismsg.go`。请求字段已核对：`user_id`、`target_id`、`channel_type`、`msg_ids`。它是异步调用，成功响应体为空。

### 历史消息

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| GET | `/apigateway/hismsgs/query` | 查询历史 | `qry_hismsgs` |
| POST | `/apigateway/hismsgs/clean` | 清空历史 | `clean_hismsg` |
| POST | `/apigateway/hismsgs/recall` | 撤回 | `recall_msg` |
| POST | `/apigateway/hismsgs/del` | 删除消息 | `del_msg` |
| POST | `/apigateway/hismsgs/modify` | 修改消息 | `modify_msg` |
| POST | `/apigateway/hismsgs/import` | 导入历史 | 私聊 `imp_pri_msg`，群 `imp_grp_msg`，都经 `upstream` |

### 全局禁言

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| POST | `/apigateway/private/globalmutemembers/add` | 加入私聊全局禁言 | `pri_global_mute` |
| POST | `/apigateway/private/globalmutemembers/del` | 解除私聊全局禁言 | `pri_global_mute` |
| GET | `/apigateway/private/globalmutemembers/query` | 查询私聊全局禁言 | `qry_pri_global_mute` |
| POST | `/apigateway/group/globalmutemembers/add` | 加入群全局禁言 | `grp_global_mute` |
| POST | `/apigateway/group/globalmutemembers/del` | 解除群全局禁言 | `grp_global_mute` |
| GET | `/apigateway/group/globalmutemembers/query` | 查询群全局禁言 | `qry_grp_global_mute` |

### 群组

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| POST | `/apigateway/groups/add` | 建群或加成员 | `g_add_members`，可选再调 `set_g_conver_conf` |
| GET | `/apigateway/groups/info` | 群信息 | `qry_group_info`，并读 `qry_g_conver_conf` |
| POST | `/apigateway/groups/update` | 更新群 | `upd_group_info`，可选再调 `set_g_conver_conf` |
| POST | `/apigateway/groups/del` | 解散群 | `g_dissolve` |
| POST | `/apigateway/groups/settings/set` | 设置群配置 | `upd_group_info` |
| GET | `/apigateway/groups/settings/get` | 读取群配置 | `qry_group_info` |
| POST | `/apigateway/groups/members/add` | 加成员 | 与 `/groups/add` 同一处理函数 |
| POST | `/apigateway/groups/members/del` | 删成员 | `g_del_members` |
| GET | `/apigateway/groups/members/query` | 查成员 | `g_qry_members` |
| POST | `/apigateway/groups/members/update` | 更新成员设置 | `set_grp_member_setting` |
| POST | `/apigateway/groups/members/querybyids` | 按 ID 查成员 | `qry_group_members_by_ids` |
| POST | `/apigateway/groups/groupmute/set` | 全员禁言 | `group_mute` |
| POST | `/apigateway/groups/groupmembermute/set` | 成员禁言 | `group_member_mute` |
| POST | `/apigateway/groups/groupmemberallow/set` | 成员白名单 | `group_member_allow` |
| POST | `/apigateway/groups/members/settings/set` | 成员个人设置 | `set_grp_member_setting` |

`/groups/add` 与 `/groups/members/add` 在原路由里绑定同一个处理函数，保持这个事实，不拆成两套语义。

### 敏感词

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| GET | `/apigateway/sensitivewords/list` | 列表 | `qry_sensitive_words` |
| POST | `/apigateway/sensitivewords/import` | 导入 | `add_sensitive_words` |
| POST | `/apigateway/sensitivewords/add` | 新增 | `add_sensitive_words` |
| POST | `/apigateway/sensitivewords/del` | 删除 | `del_sensitive_words` |

### 会话

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| POST | `/apigateway/convers/undisturb` | 免打扰 | `undisturb_convers` |
| POST | `/apigateway/convers/del` | 删除会话 | `del_convers` |
| POST | `/apigateway/convers/add` | 添加会话 | `add_conver` |
| POST | `/apigateway/convers/clearunread` | 清未读 | `clear_unread` |
| POST | `/apigateway/convers/top` | 置顶 | `top_convers` |
| GET | `/apigateway/convers/query` | 查询会话 | `qry_convers` |
| POST | `/apigateway/convers/tags/create` | 创建会话标签 | `create_user_conver_tags` |
| POST | `/apigateway/convers/tags/add` | 给会话打标签 | `tag_add_convers` |
| POST | `/apigateway/convers/tags/del` | 去掉会话标签 | `tag_del_convers` |
| GET | `/apigateway/globalconvers/query` | 查询全局会话 | `qry_global_convers` |
| POST | `/apigateway/convers/globaltags/set` | 设置全局会话标签 | `set_g_conver_conf` |
| POST | `/apigateway/convers/globaltags/get` | 读取全局会话标签 | `qry_g_conver_conf` |

### 用户标签与推送

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| GET | `/apigateway/usertags/query` | 查询用户标签 | `qry_user_tags` |
| POST | `/apigateway/usertags/add` | 添加用户标签 | `add_user_tags` |
| POST | `/apigateway/usertags/del` | 删除用户标签 | `del_user_tags` |
| POST | `/apigateway/usertags/clear` | 清空用户标签 | `clear_user_tags` |
| POST | `/apigateway/push` | 按标签推送 | `push_with_tags` |

### 好友

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| POST | `/apigateway/friends/add` | 添加好友 | `add_friends` |
| POST | `/apigateway/friends/del` | 删除好友 | `del_friends` |
| GET | `/apigateway/friends/query` | 查询好友 | `qry_friends` |
| POST | `/apigateway/friends/setdisplayname` | 设置好友备注 | `add_friends` |

设置备注复用 `add_friends`，不另造方法名。

### 绑定设备

| 方法 | 路径 | 作用 | 内部方法 |
| --- | --- | --- | --- |
| POST | `/apigateway/binddevices/add` | 绑定设备 | `add_bind_device` |
| POST | `/apigateway/binddevices/del` | 解绑设备 | `del_bind_device` |
| GET | `/apigateway/binddevices/query` | 查询绑定设备 | `qry_bind_devices` |

## 3. 导航 HTTP

默认 `http://127.0.0.1:9002`。除浏览器预检外，业务路由先走 Token 校验。

请求头：`x-appkey`、`x-token`。Token 解析后必须与 `x-appkey` 属于同一应用。

| 方法 | 路径 | 作用 | 学习顺序 |
| --- | --- | --- | --- |
| GET | `/general` | 返回当前用户可用的 WebSocket 地址 | 主线 |
| POST | `/upload-log` | 上传客户端日志 | 后置 |
| POST | `/upload-log-plain` | 上传未压缩或 gzip 客户端日志 | 后置 |
| POST | `/log-status` | 上报日志状态 | 后置 |

`GET /general` 的响应至少包含 `appKey`、`userId`、`servers`。单进程阶段 `servers` 来自配置里的默认连接地址。

导航路由在参考仓库里挂在可配置前缀下。本仓库默认前缀保持与原项目启动代码一致；实现该任务时读 `navigator` 的启动参数再把前缀写死在这里。写死之前，不另起一个新路径。

## 4. WebSocket

默认 `ws://127.0.0.1:9003`。

线上帧以参考仓库 `services/connectmanager/server/codec` 为准：二进制头携带 version、cmd、qos、checksum 和 body 长度，body 为对应 protobuf。`connect.proto` 里的 `ImWebsocketMsg` 是消息定义。实现连接任务时对照 Encode / Decode，确认头和 body 的拼法，不把 proto 的 oneof 提前当成已经核实的整帧格式。

命令编号固定：

| cmd | 名称 | 作用 |
| --- | --- | --- |
| 0 | Connect | 携带 appkey、token、设备信息，建立会话 |
| 1 | ConnectAck | 返回码、userId、session、时间戳、心跳配置 |
| 2 | Disconnect | 断开 |
| 3 | Publish | 上行。`topic` 是方法名，`targetId` 是目标 |
| 4 | PublishAck | 发布结果，含 msgId、序号、时间戳 |
| 5 | Query | 上行查询 |
| 6 | QueryAck | 查询结果 |
| 7 | QueryConfirm | 确认查询 |
| 8 | Ping | 心跳 |
| 9 | Pong | 心跳响应 |

QoS：`0` 不需要 ACK，`1` 需要 ACK。

客户端 Publish / Query 的 `topic` 必须使用 `doc/框架.md` 第 7 节里的方法名。连接管理器会把一部分历史类 topic 按频道类型重写成会话 ID 再路由，已知包括：`clean_hismsg`、`del_msg`、`del_hismsg`、`mark_read`、`modify_msg`、`qry_hismsgs`、`qry_first_unread_msg`、`qry_hismsg_by_ids`、`qry_read_infos`、`recall_msg`。私聊和系统频道重写到双方会话 ID；其他频道保持原目标。

## 5. 字段补齐规则

每实现一个接口，在本文件该接口下追加四项，并且只追加已经从原代码读到的内容：

- 请求字段和必填项
- 成功响应字段
- 该接口会返回的业务错误码
- 同步还是异步，以及路由用的目标 ID

未读到的字段写「未核对」，不填猜测值。
