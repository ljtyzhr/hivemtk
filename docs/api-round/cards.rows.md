# tiktokCard.js（#949–#956，8 条）+ xianyuCard.js（#1060–#1068，9 条）+ xiaohongshuCard.js（#1069–#1077，9 条）

> 数据：tiktok_cards 26 条（id 26 可用）、xianyu 18 条、xiaohongshu 18 条（id 29 可用）。
> 测试卡 `api26_*` 建后即删，落库 id 27/28（tiktok）、32（xianyu/xhs，各自表）。

## tiktokCard.js

| 949 | [x] | GET | `/api/tiktok-card/list` | 用户端 | query: params | 200 code=0 分页（page/page_size/keyword 生效） | `src/api/tiktokCard.js` | 4 |
| 950 | [x] | GET | `/api/tiktok-card/{id}` | 用户端 | path: id | 200 code=0（id=26，带 short_link_url）；999999 → 404「卡片不存在」 | `src/api/tiktokCard.js` | 8 |
| 951 | [x] | POST | `/api/tiktok-card` | 用户端 | body: data | 200 code=0（建 27/28） | `src/api/tiktokCard.js` | 12 |
| 952 | [x] | PUT | `/api/tiktok-card/{data_id}` | 用户端 | path: data_id / body: data | 200 code=0（标题/描述/链接/标签/is_active 全生效） | `src/api/tiktokCard.js` | 16 |
| 953 | [x] | DELETE | `/api/tiktok-card/{id}` | 用户端 | path: id | 200 code=0（27/28）；999999 → 404「记录不存在」 | `src/api/tiktokCard.js` | 20 |
| 954 | [x] | POST | `/api/tiktok-card/generate-short-link` | 用户端 | body: { card_id: cardId } | 200 code=0 `{short_code:"4Rc2q5O5",short_link_url:"/s/..."}` | `src/api/tiktokCard.js` | 24 |
| 955 | [x] | GET | `/api/tiktok-card/stats/overall` | 用户端 | query: params | 200 code=0 `{total_cards:24,active_cards:22,total_views:1,popular_cards:[...]}` | `src/api/tiktokCard.js` | 28 |
| 956 | [x] | GET | `/api/tiktok-card/{cardId}/stats` | 用户端 | path: cardId / query: params | 200 code=0（7 天 daily_stats 连续） | `src/api/tiktokCard.js` | 32 |

## xianyuCard.js

| 1060 | [x] | GET | `/api/xianyu/list` | 用户端 | query: params | 200 code=0 18 条（id 29 可用） | `src/api/xianyuCard.js` | 4 |
| 1061 | [x] | GET | `/api/xianyu/{id}` | 用户端 | path: id | 200 code=0；999999 → 404「记录不存在」 | `src/api/xianyuCard.js` | 8 |
| 1062 | [x] | POST | `/api/xianyu/create` | 用户端 | body: data | 200 code=0（建 32，**自动带短链** `/s/lX9uJK`） | `src/api/xianyuCard.js` | 12 |
| 1063 | [x] | PUT | `/api/xianyu/update` | 用户端 | body: data | 200 code=0（按 body.id 更新，字段全生效） | `src/api/xianyuCard.js` | 16 |
| 1064 | [x] | DELETE | `/api/xianyu/delete/{id}` | 用户端 | path: id | 200 code=0；999999 → 404 | `src/api/xianyuCard.js` | 20 |
| 1065 | [x] | GET | `/api/xianyu/view/{id}` | 用户端 | path: id | 200 code=0（id=29） | `src/api/xianyuCard.js` | 24 |
| 1066 | [x] | GET | `/api/xianyu/stats/card/{id}` | 用户端 | path: id / query: params | 200 code=0（chart/details/stats 三块齐全，空数据形态合法） | `src/api/xianyuCard.js` | 28 |
| 1067 | [x] | GET | `/api/xianyu/stats/overall` | 用户端 | query: params | 200 code=0 `{active_cards:18,total_cards:18,...}` | `src/api/xianyuCard.js` | 32 |
| 1068 | [x] | POST | `/api/xianyu/{id}/generate-short-link` | 用户端 | path: id | 200 code=0（重签出 `0fKUgp`，双腿各一码） | `src/api/xianyuCard.js` | 36 |

## xiaohongshuCard.js

| 1069 | [x] | GET | `/api/xiaohongshu/list` | 用户端 | query: params | 200 code=0 18 条 | `src/api/xiaohongshuCard.js` | 4 |
| 1070 | [x] | GET | `/api/xiaohongshu/{id}` | 用户端 | path: id | 200 code=0；999999 → 404「记录不存在」 | `src/api/xiaohongshuCard.js` | 8 |
| 1071 | [x] | POST | `/api/xiaohongshu/create` | 用户端 | body: data | 200 code=0（建 32，自动带短链） | `src/api/xiaohongshuCard.js` | 12 |
| 1072 | [x] | PUT | `/api/xiaohongshu/update` | 用户端 | body: data | 200 code=0（按 body.id 更新） | `src/api/xiaohongshuCard.js` | 16 |
| 1073 | [x] | DELETE | `/api/xiaohongshu/delete/{id}` | 用户端 | path: id | 200 code=0；999999 → 404 | `src/api/xiaohongshuCard.js` | 20 |
| 1074 | [x] | GET | `/api/xiaohongshu/view/{id}` | 用户端 | path: id | 200 code=0（id=29） | `src/api/xiaohongshuCard.js` | 24 |
| 1075 | [x] | GET | `/api/xiaohongshu/stats/card/{id}` | 用户端 | path: id / query: params | 200 code=0 `{cardId:32,viewCount:0,dailyStats:null}`（空数据形态） | `src/api/xiaohongshuCard.js` | 28 |
| 1076 | [x] | GET | `/api/xiaohongshu/stats/overall` | 用户端 | query: params | 200 code=0 `{totalCards:18,activeCards:18,totalViews:1,...}` | `src/api/xiaohongshuCard.js` | 32 |
| 1077 | [x] | POST | `/api/xiaohongshu/{id}/generate-short-link` | 用户端 | path: id | 200 code=0（回整卡对象 + 新码） | `src/api/xiaohongshuCard.js` | 36 |

## 备注
- 三家卡片路由形状不同但都通过：tiktok 走 `/tiktok-card/{id}` + body `{card_id}` 生成短链；
  xianyu/xiaohongshu 走 `/api/xianyu/{verb}`、update 用 body.id、生成短链用 path id。
- 一处不一致（非缺陷、记录）：xianyu/xiaohongshu 的 create 就自动发短链，tiktok 的 create 不发（需单独调 generate-short-link）。
