package gen

import (
	"github.com/yeyudekuangxiang/goctl/model/sql/template"
	"github.com/yeyudekuangxiang/goctl/util"
	"github.com/yeyudekuangxiang/goctl/util/pathx"
)

func genTableName(table Table) (string, error) {
	text, err := pathx.LoadTemplate(category, tableNameTemplateFile, template.TableName)
	if err != nil {
		return "", err
	}

	output, err := util.With("tableName").
		Parse(text).
		Execute(map[string]interface{}{
			"tableName":             table.Name.Source(),
			"upperStartCamelObject": table.Name.ToCamel(),
		})
	if err != nil {
		return "", nil
	}

	return output.String(), nil
}