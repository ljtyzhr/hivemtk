package tenancyidx

import "testing"

func TestParseTableExpr(t *testing.T) {
	cases := []struct {
		raw   string
		table string
		alias string
		ok    bool
	}{
		{"tenancy_probe", "tenancy_probe", "", true},
		{`"tenancy_probe"`, "tenancy_probe", "", true},
		{"tenancy_probe AS tp", "tenancy_probe", "tp", true},
		{"tenancy_probe tp", "tenancy_probe", "tp", true},
		{`public."tenancy_probe" AS tp`, "tenancy_probe", "tp", true},
		{"ONLY tenancy_probe", "tenancy_probe", "", true},
		{"(?) AS u", "", "", false},
		{"tenancy_probe AS WHERE", "", "", false},
	}
	for _, c := range cases {
		ref, ok := parseTableExpr(c.raw)
		if ok != c.ok || ref.table != c.table || ref.alias != c.alias {
			t.Errorf("parseTableExpr(%q) = (%+v,%v), want (%q,%q,%v)", c.raw, ref, ok, c.table, c.alias, c.ok)
		}
	}
}

func TestScanTableRefs(t *testing.T) {
	cases := []struct {
		sql   string
		table string
		alias string
	}{
		{"JOIN tenancy_probe AS tp ON tp.id = g.id", "tenancy_probe", "tp"},
		{"JOIN tenancy_probe tp ON tp.id = g.id", "tenancy_probe", "tp"},
		{`SELECT count(*) FROM "tenancy_probe"`, "tenancy_probe", ""},
		{"SELECT count(*) FROM ONLY tenancy_probe", "tenancy_probe", ""},
		{"SELECT count(*) FROM public.tenancy_probe WHERE x = 1", "tenancy_probe", ""},
		{"TRUNCATE TABLE tenancy_probe", "tenancy_probe", ""},
		{"DROP TABLE IF EXISTS tenancy_probe", "tenancy_probe", ""},
		{"INSERT INTO tenancy_probe (name) VALUES ('x')", "tenancy_probe", ""},
		{"COPY tenancy_probe FROM STDIN", "tenancy_probe", ""},
		{"ALTER TABLE tenancy_probe ADD COLUMN x int", "tenancy_probe", ""},
	}
	for _, c := range cases {
		refs := scanTableRefs(c.sql)
		found := false
		for _, r := range refs {
			if r.table == c.table {
				found = true
				if r.alias != c.alias {
					t.Errorf("scanTableRefs(%q) 表 %s 别名 = %q, want %q", c.sql, c.table, r.alias, c.alias)
				}
			}
		}
		if !found {
			t.Errorf("scanTableRefs(%q) 未识别 %s，实际 %+v", c.sql, c.table, refs)
		}
	}
}
