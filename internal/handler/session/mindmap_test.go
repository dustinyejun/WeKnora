package session

import "testing"

func TestParseMindmapTree(t *testing.T) {
	ok := "好的：\n```json\n{\"title\":\"内控\",\"children\":[{\"title\":\"目标\",\"children\":[{\"title\":\"合规\"}]}]}\n```"
	tree, valid := parseMindmapTree(ok)
	if !valid || tree.Title != "内控" || len(tree.Children) != 1 || tree.Children[0].Children[0].Title != "合规" {
		t.Fatalf("valid tree rejected or mangled: %+v ok=%v", tree, valid)
	}

	cases := map[string]string{
		"no json":     "mindmap\n  root((A))",
		"empty title": `{"title":"","children":[{"title":"x"}]}`,
		"no children": `{"title":"only root"}`,
		"too deep":    `{"title":"a","children":[{"title":"b","children":[{"title":"c","children":[{"title":"d","children":[{"title":"e","children":[{"title":"f","children":[{"title":"g"}]}]}]}]}]}]}`,
		"blank child": `{"title":"a","children":[{"title":"  "}]}`,
	}
	for name, raw := range cases {
		if _, valid := parseMindmapTree(raw); valid {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestParseMindmapTreeStripsCitationTags(t *testing.T) {
	raw := `{"title":"内控","children":[{"title":"资产控制 <kb doc=\"规范.pdf\" chunk_id=\"c1\" /> 货币资金分离"},{"title":"采购"}]}`
	tree, ok := parseMindmapTree(raw)
	if !ok {
		t.Fatal("tree rejected")
	}
	if got := tree.Children[0].Title; got != "资产控制  货币资金分离" && got != "资产控制 货币资金分离" {
		t.Fatalf("citation tag leaked into title: %q", got)
	}
}
