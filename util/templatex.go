package util

import (
	"bytes"
	"encoding/json"
	"github.com/yeyudekuangxiang/goctl/util/ctx"
	goformat "go/format"
	"io/ioutil"
	"path/filepath"
	"text/template"

	"github.com/yeyudekuangxiang/goctl/internal/errorx"
	"github.com/yeyudekuangxiang/goctl/util/pathx"
)

const regularPerm = 0o666

// DefaultTemplate is a tool to provides the text/template operations
type DefaultTemplate struct {
	name  string
	text  string
	goFmt bool
}

// With returns a instance of DefaultTemplate
func With(name string) *DefaultTemplate {
	return &DefaultTemplate{
		name: name,
	}
}

// Parse accepts a source template and returns DefaultTemplate
func (t *DefaultTemplate) Parse(text string) *DefaultTemplate {
	t.text = text
	return t
}

// GoFmt sets the value to goFmt and marks the generated codes will be formatted or not
func (t *DefaultTemplate) GoFmt(format bool) *DefaultTemplate {
	t.goFmt = format
	return t
}

func projectInfo(dir string) (*ctx.ProjectContext, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	return ctx.Prepare(abs)
}

// SaveTo writes the codes to the target path
func (t *DefaultTemplate) SaveTo(data interface{}, path string, forceUpdate bool) error {
	if pathx.FileExists(path) && !forceUpdate {
		return nil
	}

	data2 := make(map[string]interface{})
	d, err := json.Marshal(data)
	if err != nil {
		return err
	}
	err = json.Unmarshal(d, &data2)
	if err != nil {
		return err
	}

	projectInfo, err := projectInfo(filepath.Dir(path))
	if err != nil {
		return err
	}

	data2["projectWorkDir"] = projectInfo.WorkDir
	data2["projectName"] = projectInfo.Name
	data2["projectPath"] = projectInfo.Path
	data2["projectDir"] = projectInfo.Dir

	output, err := t.Execute(data2)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, output.Bytes(), regularPerm)
}

// Execute returns the codes after the template executed
func (t *DefaultTemplate) Execute(data interface{}) (*bytes.Buffer, error) {
	tem, err := template.New(t.name).Parse(t.text)
	if err != nil {
		return nil, errorx.Wrap(err, "template parse error:", t.text)
	}

	buf := new(bytes.Buffer)
	if err = tem.Execute(buf, data); err != nil {
		return nil, errorx.Wrap(err, "template execute error:", t.text)
	}

	if !t.goFmt {
		return buf, nil
	}

	formatOutput, err := goformat.Source(buf.Bytes())
	if err != nil {
		return nil, errorx.Wrap(err, "go format error:", buf.String())
	}

	buf.Reset()
	buf.Write(formatOutput)
	return buf, nil
}
