package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 筛选下推的契约测试。
//
// 这一层曾整块空转：repository/clue_query.go 早就写好了按 type / is_verify / keyword /
// 时间区间过滤的 SQL，但 GetClueRequest 只绑 page/limit，controller 走的是不分条件的
// GetClueList —— 于是页面上的筛选只在前端已加载的那 10 行里再挑一遍，
// 选中库里真有 29 条的类型会显示「暂无线索数据」，而分页条同时写着「共 214 条」。
//
// 所以这里从 HTTP 查询串一路打到返回体，三段都要钉：
// 参数名与指针绑定（is_verify=0 是"筛未验证"，不是"没传"）、过滤真的进了 SQL、
// 返回体带上跟进人/创建时间/最后跟进（前端两列曾因缺字段而恒为 '-'）。

type clueListEnvelope struct {
	Code int `json:"code"`
	Data struct {
		Total int64                  `json:"total"`
		List  []dto.ClueListResponse `json:"list"`
	} `json:"data"`
}

func clueGetList(t *testing.T, query string) (clueListEnvelope, string) {
	t.Helper()
	router := gin.New()
	router.GET("/clues", NewClueController().GetClueList)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/clues"+query, nil))
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("GET /clues%s ⇒ HTTP %d body=%s", query, w.Code, body)
	}
	var env clueListEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("返回体解析失败: %v body=%s", err, body)
	}
	if env.Code != 0 {
		t.Fatalf("业务码=%d body=%s", env.Code, body)
	}
	return env, body
}

func seedClueFilterRows(t *testing.T, database *gorm.DB) {
	t.Helper()
	// create_time / updated_at 显式给值：autoCreateTime 只在零值时填充，
	// 而时间区间那几格的期望值要靠可比的已知刻度算出来。
	base := int64(1790000000)
	rows := []*model.Clue{
		{SourceID: "s1", Account: "tg-a", Name: "Telegram 客户 A", City: "深圳", Type: 4, IsVerify: 1, OwnerAccount: "alice", CreateTime: base + 100, UpdatedAt: base + 150},
		{SourceID: "s1", Account: "tg-b", Name: "Telegram 客户 B", City: "", Type: 4, IsVerify: 0, CreateTime: base + 200, UpdatedAt: base + 200},
		{SourceID: "s2", Account: "dy-a", Name: "北京抖音客户", City: "北京", Type: 9, IsVerify: 0, CreateTime: base + 300, UpdatedAt: base + 300},
		{SourceID: "s2", Account: "dy-b", Name: "抖音小号", City: "", Type: 9, IsVerify: 0, CreateTime: base + 400, UpdatedAt: base + 400},
		{SourceID: "s3", Account: "x&y", Name: "带分隔符的名称", City: "", Type: 12, IsVerify: 0, CreateTime: base + 500, UpdatedAt: base + 500},
	}
	for _, r := range rows {
		if err := database.Create(r).Error; err != nil {
			t.Fatalf("夹具写入失败: %v", err)
		}
	}
}

// clueAccounts 取出返回行的账号集合（排序后）。
// 光比 total 与行数会漏掉一整类错误：过滤方向反了（>= 写成 <=）在对称夹具下
// 返回的行数一模一样，只有"是哪几行"能把它区分开。
func clueAccounts(list []dto.ClueListResponse) []string {
	got := make([]string, 0, len(list))
	for _, r := range list {
		got = append(got, r.Account)
	}
	sort.Strings(got)
	return got
}

func TestClueController_GetClueList_FilterPushdown(t *testing.T) {
	database := setupClueTestDB(t)
	seedClueFilterRows(t, database)

	cases := []struct {
		name     string
		query    string
		want     int64
		accounts []string
	}{
		{"无条件＝全量", "?page=1&limit=10", 5, []string{"dy-a", "dy-b", "tg-a", "tg-b", "x&y"}},
		{"按类型 4", "?page=1&limit=10&type=4", 2, []string{"tg-a", "tg-b"}},
		{"按类型 9", "?page=1&limit=10&type=9", 2, []string{"dy-a", "dy-b"}},
		{"按类型 12", "?page=1&limit=10&type=12", 1, []string{"x&y"}},
		// 夹具里没有 type=14：0 是"筛出来就是空"，与"没传 type"（5 条）必须区分开
		{"按类型 14（无命中）", "?page=1&limit=10&type=14", 0, nil},
		{"未验证＝is_verify 的 0 不是没传", "?page=1&limit=10&is_verify=0", 4, []string{"dy-a", "dy-b", "tg-b", "x&y"}},
		{"已验证", "?page=1&limit=10&is_verify=1", 1, []string{"tg-a"}},
		{"类型+验证组合", "?page=1&limit=10&type=4&is_verify=1", 1, []string{"tg-a"}},
		{"关键字命中城市", "?page=1&limit=10&keyword=北京", 1, []string{"dy-a"}},
		// 只给字段的一部分：整值命中的那几格在"模糊退成精确"时照样绿，
		// 片段才证明 SQL 里那两个 % 还在。
		{"关键字命中城市片段", "?page=1&limit=10&keyword=圳", 1, []string{"tg-a"}},
		{"关键字命中跟进人片段", "?page=1&limit=10&keyword=ice", 1, []string{"tg-a"}},
		{"关键字命中账号/名称", "?page=1&limit=10&keyword=telegram", 2, []string{"tg-a", "tg-b"}},
		{"关键字命中跟进人", "?page=1&limit=10&keyword=alice", 1, []string{"tg-a"}},
		{"关键字大小写不敏感", "?page=1&limit=10&keyword=TELEGRAM", 2, []string{"tg-a", "tg-b"}},
		{"时间下界", "?page=1&limit=10&start=1790000300", 3, []string{"dy-a", "dy-b", "x&y"}},
		{"时间上界", "?page=1&limit=10&end=1790000200", 2, []string{"tg-a", "tg-b"}},
		{"时间区间", "?page=1&limit=10&start=1790000200&end=1790000400", 3, []string{"dy-a", "dy-b", "tg-b"}},
		{"区间外＝空", "?page=1&limit=10&start=1790000000&end=1790000050", 0, nil},
		{"类型+时间区间", "?page=1&limit=10&type=9&start=1790000350", 1, []string{"dy-b"}},
	}
	for _, c := range cases {
		got, _ := clueGetList(t, c.query)
		if got.Data.Total != c.want {
			t.Errorf("%s: total=%d 期望 %d（query=%s）", c.name, got.Data.Total, c.want, c.query)
		}
		rows := int64(len(got.Data.List))
		if want := c.want; want < 10 && rows != want {
			t.Errorf("%s: 行数=%d 期望 %d", c.name, rows, want)
		} else if want >= 10 && rows != 10 {
			t.Errorf("%s: 行数=%d 期望 10（limit=10）", c.name, rows)
		}
		if gotRows := clueAccounts(got.Data.List); !reflect.DeepEqual(gotRows, c.accounts) && !(len(gotRows) == 0 && len(c.accounts) == 0) {
			t.Errorf("%s: 命中的是 %v 期望 %v", c.name, gotRows, c.accounts)
		}
	}

	t.Run("查询串里的 & 不会被劈成两个参数", func(t *testing.T) {
		// 前端用 URLSearchParams 编码成 keyword=x%26y；若退化成手工拼串，
		// 服务端只会收到 keyword=x ⇒ 命中 0 条，而真值是 1 条（账号就叫 x&y）。
		got, _ := clueGetList(t, "?page=1&limit=10&keyword="+url.QueryEscape("x&y"))
		if got.Data.Total != 1 {
			t.Errorf("keyword=x&y: total=%d 期望 1（若未编码，服务端只收到 keyword=x）", got.Data.Total)
		}
	})

	t.Run("返回体带上前端在用的三列", func(t *testing.T) {
		_, body := clueGetList(t, "?page=1&limit=10&keyword=alice")
		var raw struct {
			Data struct {
				List []map[string]any `json:"list"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &raw); err != nil {
			t.Fatal(err)
		}
		if len(raw.Data.List) != 1 {
			t.Fatalf("keyword=alice 期望 1 行，实得 %d", len(raw.Data.List))
		}
		row := raw.Data.List[0]
		for _, k := range []string{"owner_account", "create_time", "updated_at"} {
			if _, ok := row[k]; !ok {
				t.Errorf("返回体缺 %q：前端那一列会恒为 '-'", k)
			}
		}
		if row["owner_account"] != "alice" {
			t.Errorf("owner_account=%v 期望 alice", row["owner_account"])
		}
		// unix 秒：若这里给成毫秒，前端 *1000 后会显示 5w 年后
		if ct, _ := row["create_time"].(float64); int64(ct) != 1790000100 {
			t.Errorf("create_time=%v 期望夹具写入的 unix 秒 1790000100", row["create_time"])
		}
		if ut, _ := row["updated_at"].(float64); int64(ut) != 1790000150 {
			t.Errorf("updated_at=%v 期望夹具写入的 1790000150（与 create_time 不同值，才能证明没串字段）", row["updated_at"])
		}
	})

	t.Run("limit 生效且总数不受页容量影响", func(t *testing.T) {
		got, _ := clueGetList(t, "?page=1&limit=2")
		if got.Data.Total != 5 {
			t.Errorf("total=%d 期望 5", got.Data.Total)
		}
		if len(got.Data.List) != 2 {
			t.Errorf("行数=%d 期望 2", len(got.Data.List))
		}
	})

	t.Run("按 create_time 倒序，翻页不重不漏", func(t *testing.T) {
		var all []dto.ClueListResponse
		for _, p := range []string{"?page=1&limit=2", "?page=2&limit=2", "?page=3&limit=2"} {
			got, _ := clueGetList(t, p)
			all = append(all, got.Data.List...)
		}
		seen := map[string]bool{}
		for _, r := range all {
			if seen[r.ID] {
				t.Errorf("翻页重复返回了 %s", r.ID)
			}
			seen[r.ID] = true
		}
		if len(seen) != 5 {
			t.Errorf("三页合计 %d 行，期望覆盖全部 5 条", len(seen))
		}
		var prev int64 = 1 << 62
		for _, r := range all {
			if r.CreateTime > prev {
				t.Fatalf("create_time 不是倒序：%d 排在 %d 之后", r.CreateTime, prev)
			}
			prev = r.CreateTime
		}
	})

	t.Run("非法 type 不被静默当成没传", func(t *testing.T) {
		router := gin.New()
		router.GET("/clues", NewClueController().GetClueList)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/clues?page=1&limit=10&type=abc", nil))
		// 判据只看一件事：不能给回全量 5 条。400 或业务码非 0 都算拦住了。
		var env clueListEnvelope
		if w.Code == http.StatusOK && json.Unmarshal(w.Body.Bytes(), &env) == nil &&
			env.Code == 0 && env.Data.Total == 5 {
			t.Errorf("type=abc 被静默当成没传，返回全量 5 条")
		}
	})
}
