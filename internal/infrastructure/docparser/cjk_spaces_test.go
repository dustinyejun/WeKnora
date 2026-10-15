package docparser

import "testing"

func TestJoinCJKSpaces(t *testing.T) {
	cases := []struct{ in, want string }{
		{"对信息系统建设实施归 口管理， 将经济活动嵌入单 位信息系统中",
			"对信息系统建设实施归口管理，将经济活动嵌入单位信息系统中"},
		{"第十八条 单位应当充分运用", "第十八条 单位应当充分运用"},
		{"第一章 总则", "第一章 总则"},
		{"第3条 内容", "第3条 内容"},
		{"使用 WeKnora 平台", "使用 WeKnora 平台"},
		{"共 65 条", "共 65 条"},
		{"| 名称 | 说明 |", "| 名称 | 说明 |"},
		{"第一行\n第二行", "第一行\n第二行"},
		{"姓\u3000\u3000名", "姓名"},
		{"", ""},
	}
	for _, c := range cases {
		if got := JoinCJKSpaces(c.in); got != c.want {
			t.Errorf("JoinCJKSpaces(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJoinCJKLineWraps(t *testing.T) {
	cases := []struct{ in, want string }{
		{"将经济活动及其内部控制流程嵌入单\n位信息系统中", "将经济活动及其内部控制流程嵌入单位信息系统中"},
		{"对经济活动的决策、执\n行和监督全过程", "对经济活动的决策、执行和监督全过程"},
		{"风险评估和控制方法\n第八条 单位应当", "风险评估和控制方法\n第八条 单位应当"},
		{"进行重估。\n第九条", "进行重估。\n第九条"},
		{"重点关注以下方\n面：\n（一）内部控制", "重点关注以下方面：\n（一）内部控制"},
		{"## 第一章 总则\n第一条", "## 第一章 总则\n第一条"},
		{"## 标题\n正文", "## 标题\n正文"},
		{"| 名称 |\n说明", "| 名称 |\n说明"},
		{"第一段\n\n第二段", "第一段\n\n第二段"},
		{"English\nline", "English\nline"},
	}
	for _, c := range cases {
		if got := JoinCJKLineWraps(c.in); got != c.want {
			t.Errorf("JoinCJKLineWraps(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
