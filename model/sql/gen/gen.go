package gen

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yeyudekuangxiang/goctl/config"
	"github.com/yeyudekuangxiang/goctl/model/sql/model"
	"github.com/yeyudekuangxiang/goctl/model/sql/parser"
	"github.com/yeyudekuangxiang/goctl/model/sql/template"
	modelutil "github.com/yeyudekuangxiang/goctl/model/sql/util"
	"github.com/yeyudekuangxiang/goctl/util"
	"github.com/yeyudekuangxiang/goctl/util/console"
	"github.com/yeyudekuangxiang/goctl/util/format"
	"github.com/yeyudekuangxiang/goctl/util/pathx"
	"github.com/yeyudekuangxiang/goctl/util/stringx"
)

const pwd = "."

type (
	defaultGenerator struct {
		appName string
		console.Console
		// source string
		dir          string
		pkg          string
		cfg          *config.Config
		isPostgreSql bool
	}

	// Option defines a function with argument defaultGenerator
	Option func(generator *defaultGenerator)

	code struct {
		importsCode string
		varsCode    string
		typesCode   string
		newCode     string
		insertCode  string
		findCode    []string
		updateCode  string
		deleteCode  string
		cacheExtra  string
		tableName   string
	}

	codeTuple struct {
		modelCode       string
		modelCustomCode string
	}
)

// NewDefaultGenerator creates an instance for defaultGenerator
func NewDefaultGenerator(appName string, dir string, cfg *config.Config, opt ...Option) (*defaultGenerator, error) {
	if dir == "" {
		dir = pwd
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	dir = dirAbs
	pkg := util.SafeString(filepath.Base(dirAbs))
	err = pathx.MkdirIfNotExist(dir)
	if err != nil {
		return nil, err
	}

	generator := &defaultGenerator{appName: appName, dir: dir, cfg: cfg, pkg: pkg}
	var optionList []Option
	optionList = append(optionList, newDefaultOption())
	optionList = append(optionList, opt...)
	for _, fn := range optionList {
		fn(generator)
	}

	return generator, nil
}

// WithConsoleOption creates a console option
func WithConsoleOption(c console.Console) Option {
	return func(generator *defaultGenerator) {
		generator.Console = c
	}
}

// WithPostgreSql marks  defaultGenerator.isPostgreSql true
func WithPostgreSql() Option {
	return func(generator *defaultGenerator) {
		generator.isPostgreSql = true
	}
}

func newDefaultOption() Option {
	return func(generator *defaultGenerator) {
		generator.Console = console.NewColorConsole()
	}
}

func (g *defaultGenerator) StartFromDDL(filename string, withCache bool, database string) error {
	modelList, err := g.genFromDDL(filename, withCache, database)
	if err != nil {
		return err
	}

	return g.createFile(modelList)
}

func (g *defaultGenerator) StartFromInformationSchema(tables map[string]*model.Table, withCache bool) error {
	m := make(map[string]*codeTuple)
	for _, each := range tables {
		table, err := parser.ConvertDataType(each)
		if err != nil {
			return err
		}

		code, err := g.genModel(*table, withCache)
		if err != nil {
			return err
		}
		customCode, err := g.genModelCustom(*table, withCache)
		if err != nil {
			return err
		}

		m[table.Name.Source()] = &codeTuple{
			modelCode:       code,
			modelCustomCode: customCode,
		}
	}

	return g.createFile(m)
}

func (g *defaultGenerator) createFile(modelList map[string]*codeTuple) error {
	dirAbs, err := filepath.Abs(g.dir)
	if err != nil {
		return err
	}

	g.dir = dirAbs
	g.pkg = filepath.Base(dirAbs)
	err = pathx.MkdirIfNotExist(dirAbs)
	if err != nil {
		return err
	}

	for tableName, codes := range modelList {
		tn := stringx.From(tableName)
		modelFilename, err := format.FileNamingFormat(g.cfg.NamingFormat,
			fmt.Sprintf("%s_model", tn.Source()))
		if err != nil {
			return err
		}

		name := util.SafeString(modelFilename) + "_gen.go"
		filename := filepath.Join(dirAbs, name)
		err = ioutil.WriteFile(filename, []byte(codes.modelCode), os.ModePerm)
		if err != nil {
			return err
		}

		name = util.SafeString(modelFilename) + ".go"
		filename = filepath.Join(dirAbs, name)
		if pathx.FileExists(filename) {
			g.Warning("%s already exists, ignored.", name)
			continue
		}
		err = ioutil.WriteFile(filename, []byte(codes.modelCustomCode), os.ModePerm)
		if err != nil {
			return err
		}
	}

	// generate error file

	filename := filepath.Join(dirAbs, "repo.go")
	text, err := pathx.LoadTemplate(category, repoTemplateFile, "")
	if err != nil {
		return err
	}

	err = util.With("repo").Parse(text).SaveTo(map[string]interface{}{
		"pkg":     g.pkg,
		"appName": toCaml(g.appName),
	}, filename, false)
	if err != nil {
		return err
	}

	// generate error file

	filename = filepath.Join(dirAbs, "repo_gen.go")
	text, err = pathx.LoadTemplate(category, repoGenTemplateFile, "")
	if err != nil {
		return err
	}

	models := getModels(dirAbs)

	err = util.With("repo").Parse(text).SaveTo(map[string]interface{}{
		"pkg":       g.pkg,
		"appName":   toCaml(g.appName),
		"models":    importModels(models),
		"newModels": newModels(models),
	}, filename, true)
	if err != nil {
		return err
	}

	// generate error file
	varFilename, err := format.FileNamingFormat(g.cfg.NamingFormat, "vars")
	if err != nil {
		return err
	}

	filename = filepath.Join(dirAbs, varFilename+".go")
	text, err = pathx.LoadTemplate(category, errTemplateFile, template.Error)
	if err != nil {
		return err
	}

	err = util.With("vars").Parse(text).SaveTo(map[string]interface{}{
		"pkg": g.pkg,
	}, filename, false)
	if err != nil {
		return err
	}

	g.Success("Done.")
	return nil
}

type repoModel struct {
	upperName string
	withCache bool
}

func getModels(modelPath string) []repoModel {
	list, err := os.ReadDir(modelPath)
	if err != nil {
		return nil
	}
	models := make([]repoModel, 0)
	for _, f := range list {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(f.Name(), "model.go") {
			data, err := ioutil.ReadFile(path.Join(modelPath, f.Name()))
			if err != nil {
				console.Error("读取文件内容异常 %s %v", path.Join(modelPath, f.Name()), err)
				continue
			}
			cacheReg := regexp.MustCompile("func New.*?Model.*?CacheConf")
			modelFunc := string(cacheReg.Find(data))
			if len(modelFunc) > 0 {
				models = append(models, repoModel{
					upperName: modelFunc[8:strings.Index(modelFunc, "(")],
					withCache: true,
				})
				continue
			}
			reg := regexp.MustCompile("func New.*?Model")
			modelFunc = string(reg.Find(data))
			if len(modelFunc) > 0 {
				models = append(models, repoModel{
					upperName: modelFunc[8:],
					withCache: false,
				})
				continue
			}
		}
	}
	return models
}
func importModels(models []repoModel) string {
	str := ""
	for _, m := range models {
		str += m.upperName + " " + m.upperName + "\n"
	}
	if len(str) > 0 {
		str = strings.Trim(str, "\n")
	}
	return str
}
func newModels(models []repoModel) string {
	str := ""
	for _, m := range models {
		if m.withCache {
			str += m.upperName + ":New" + m.upperName + "(db,c)" + ",\n"
		} else {
			str += m.upperName + ":New" + m.upperName + "(db)" + ",\n"
		}
	}
	if len(str) > 0 {
		str = strings.Trim(str, "\n")
	}
	return str
}
func toCaml(str string) string {
	str = strings.ToLower(str)
	reg, _ := regexp.Compile("[-_]+([a-z]|/d)")

	str = reg.ReplaceAllStringFunc(str, func(s string) string {
		return strings.ToUpper(s[len(s)-1:])
	})
	return strings.ToUpper(str[:1]) + str[1:]
}

// ret1: key-table name,value-code
func (g *defaultGenerator) genFromDDL(filename string, withCache bool, database string) (
	map[string]*codeTuple, error,
) {
	m := make(map[string]*codeTuple)
	tables, err := parser.Parse(filename, database)
	if err != nil {
		return nil, err
	}

	for _, e := range tables {
		code, err := g.genModel(*e, withCache)
		if err != nil {
			return nil, err
		}
		customCode, err := g.genModelCustom(*e, withCache)
		if err != nil {
			return nil, err
		}

		m[e.Name.Source()] = &codeTuple{
			modelCode:       code,
			modelCustomCode: customCode,
		}
	}

	return m, nil
}

// Table defines mysql table
type Table struct {
	parser.Table
	PrimaryCacheKey        Key
	UniqueCacheKey         []Key
	ContainsUniqueCacheKey bool
}

func (g *defaultGenerator) genModel(in parser.Table, withCache bool) (string, error) {
	if len(in.PrimaryKey.Name.Source()) == 0 {
		return "", fmt.Errorf("table %s: missing primary key", in.Name.Source())
	}

	primaryKey, uniqueKey := genCacheKeys(in)

	var table Table
	table.Table = in
	table.PrimaryCacheKey = primaryKey
	table.UniqueCacheKey = uniqueKey
	table.ContainsUniqueCacheKey = len(uniqueKey) > 0

	importsCode, err := genImports(table, withCache, in.ContainsTime())
	if err != nil {
		return "", err
	}

	varsCode, err := genVars(table, withCache, g.isPostgreSql)
	if err != nil {
		return "", err
	}

	insertCode, insertCodeMethod, err := genInsert(table, withCache, g.isPostgreSql)
	if err != nil {
		return "", err
	}

	findCode := make([]string, 0)
	findOneCode, findOneCodeMethod, err := genFindOne(table, withCache, g.isPostgreSql)
	if err != nil {
		return "", err
	}

	ret, err := genFindOneByField(table, withCache, g.isPostgreSql)
	if err != nil {
		return "", err
	}

	findCode = append(findCode, findOneCode, ret.findOneMethod)
	updateCode, updateCodeMethod, err := genUpdate(table, withCache, g.isPostgreSql)
	if err != nil {
		return "", err
	}

	deleteCode, deleteCodeMethod, err := genDelete(table, withCache, g.isPostgreSql)
	if err != nil {
		return "", err
	}

	var list []string
	list = append(list, insertCodeMethod, findOneCodeMethod, ret.findOneInterfaceMethod,
		updateCodeMethod, deleteCodeMethod)
	typesCode, err := genTypes(table, strings.Join(modelutil.TrimStringSlice(list), pathx.NL), withCache)
	if err != nil {
		return "", err
	}

	newCode, err := genNew(table, withCache, g.isPostgreSql)
	if err != nil {
		return "", err
	}

	tableName, err := genTableName(table)
	if err != nil {
		return "", err
	}

	code := &code{
		importsCode: importsCode,
		varsCode:    varsCode,
		typesCode:   typesCode,
		newCode:     newCode,
		insertCode:  insertCode,
		findCode:    findCode,
		updateCode:  updateCode,
		deleteCode:  deleteCode,
		cacheExtra:  ret.cacheExtra,
		tableName:   tableName,
	}

	output, err := g.executeModel(table, code)
	if err != nil {
		return "", err
	}

	return output.String(), nil
}

func (g *defaultGenerator) genModelCustom(in parser.Table, withCache bool) (string, error) {
	text, err := pathx.LoadTemplate(category, modelCustomTemplateFile, template.ModelCustom)
	if err != nil {
		return "", err
	}

	t := util.With("model-custom").
		Parse(text).
		GoFmt(true)
	hasCreatedAt := false
	hasUpdatedAt := false
	for _, f := range in.Fields {
		if f.NameOriginal == "created_at" {
			hasCreatedAt = true
		}
		if f.NameOriginal == "updated_at" {
			hasUpdatedAt = true
		}
	}
	output, err := t.Execute(map[string]interface{}{
		"pkg":                       g.pkg,
		"withCache":                 withCache,
		"upperStartCamelObject":     in.Name.ToCamel(),
		"lowerStartCamelObject":     stringx.From(in.Name.ToCamel()).Untitle(),
		"upperStartCamelPrimaryKey": in.PrimaryKey.Name.ToCamel(),
		"originPrimaryKey":          in.PrimaryKey.Name.Source(),
		"hasCreatedAt":              hasCreatedAt,
		"hasUpdatedAt":              hasUpdatedAt,
	})
	if err != nil {
		return "", err
	}

	return output.String(), nil
}

func (g *defaultGenerator) executeModel(table Table, code *code) (*bytes.Buffer, error) {
	text, err := pathx.LoadTemplate(category, modelGenTemplateFile, template.ModelGen)
	if err != nil {
		return nil, err
	}
	t := util.With("model").
		Parse(text).
		GoFmt(true)
	output, err := t.Execute(map[string]interface{}{
		"pkg":         g.pkg,
		"imports":     code.importsCode,
		"vars":        code.varsCode,
		"types":       code.typesCode,
		"new":         code.newCode,
		"insert":      code.insertCode,
		"find":        strings.Join(code.findCode, "\n"),
		"update":      code.updateCode,
		"delete":      code.deleteCode,
		"extraMethod": code.cacheExtra,
		"tableName":   code.tableName,
		"data":        table,
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

func wrapWithRawString(v string, postgreSql bool) string {
	if postgreSql {
		return v
	}

	if v == "`" {
		return v
	}

	if !strings.HasPrefix(v, "`") {
		v = "`" + v
	}

	if !strings.HasSuffix(v, "`") {
		v = v + "`"
	} else if len(v) == 1 {
		v = v + "`"
	}

	return v
}
