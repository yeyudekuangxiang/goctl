package generator

import (
	_ "embed"
	"fmt"
	"path/filepath"

	conf "github.com/yeyudekuangxiang/goctl/config"
	"github.com/yeyudekuangxiang/goctl/rpc/parser"
	"github.com/yeyudekuangxiang/goctl/util"
	"github.com/yeyudekuangxiang/goctl/util/format"
	"github.com/yeyudekuangxiang/goctl/util/pathx"
)

//go:embed svc.tpl
var svcTemplate string

// GenSvc generates the servicecontext.go file, which is the resource dependency of a service,
// such as rpc dependency, model dependency, etc.
func (g *Generator) GenSvc(ctx DirContext, _ parser.Proto, cfg *conf.Config) error {
	dir := ctx.GetSvc()
	svcFilename, err := format.FileNamingFormat(cfg.NamingFormat, "service_context")
	if err != nil {
		return err
	}

	fileName := filepath.Join(dir.Filename, svcFilename+".go")
	text, err := pathx.LoadTemplate(category, svcTemplateFile, svcTemplate)
	if err != nil {
		return err
	}

	return util.With("svc").GoFmt(true).Parse(text).SaveTo(map[string]interface{}{
		"imports": fmt.Sprintf(`"%v"`, ctx.GetConfig().Package),
	}, fileName, false)
}
