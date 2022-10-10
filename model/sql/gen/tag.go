package gen

import (
	"github.com/yeyudekuangxiang/goctl/model/sql/template"
	"github.com/yeyudekuangxiang/goctl/util"
	"github.com/yeyudekuangxiang/goctl/util/pathx"
)

func genTag(table Table, in string) (string, error) {
	if in == "" {
		return in, nil
	}

	text, err := pathx.LoadTemplate(category, tagTemplateFile, template.Tag)
	if err != nil {
		return "", err
	}

	isPk := table.PrimaryKey.Name.Source() == in
	output, err := util.With("tag").Parse(text).Execute(map[string]interface{}{
		"isPk":            isPk,
		"isAutoIncrement": isPk && table.PrimaryKey.AutoIncrement,
		"field":           in,
		"data":            table,
	})
	if err != nil {
		return "", err
	}

	return output.String(), nil
}
