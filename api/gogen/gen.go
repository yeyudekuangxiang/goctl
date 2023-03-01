package gogen

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	apiformat "github.com/yeyudekuangxiang/goctl/api/format"
	"github.com/yeyudekuangxiang/goctl/api/parser"
	apiutil "github.com/yeyudekuangxiang/goctl/api/util"
	"github.com/yeyudekuangxiang/goctl/config"
	"github.com/yeyudekuangxiang/goctl/pkg/golang"
	"github.com/yeyudekuangxiang/goctl/util"
	"github.com/yeyudekuangxiang/goctl/util/pathx"
	"github.com/zeromicro/go-zero/core/logx"
)

const tmpFile = "%s-%d"

var (
	tmpDir = path.Join(os.TempDir(), "goctl")
	// VarStringDir describes the directory.
	VarStringDir string
	// VarStringAPI describes the API.
	VarStringAPI string
	// VarStringMiddleware describes the middleware.
	VarStringMiddleware string
	// VarStringHome describes the go home.
	VarStringHome string
	// VarStringRemote describes the remote git repository.
	VarStringRemote string
	// VarStringBranch describes the branch.
	VarStringBranch string
	// VarStringStyle describes the style of output files.
	VarStringStyle string
)

// GoCommand gen go project files from command line
func GoCommand(_ *cobra.Command, _ []string) error {
	apiFile := VarStringAPI
	dir := VarStringDir
	jwtMiddlewareDir := VarStringMiddleware

	namingStyle := VarStringStyle
	home := VarStringHome
	remote := VarStringRemote
	branch := VarStringBranch
	if len(remote) > 0 {
		repo, _ := util.CloneIntoGitHome(remote, branch)
		if len(repo) > 0 {
			home = repo
		}
	}

	if len(home) > 0 {
		pathx.RegisterGoctlHome(home)
	}
	if len(apiFile) == 0 {
		return errors.New("missing -api")
	}
	if len(dir) == 0 {
		return errors.New("missing -dir")
	}

	return DoGenProject(apiFile, dir, namingStyle, jwtMiddlewareDir)
}

// DoGenProject gen go project files with api file
func DoGenProject(apiFile, dir, style, jwtMiddlewareDir string) error {
	api, err := parser.Parse(apiFile)
	if err != nil {
		return err
	}

	if err := api.Validate(); err != nil {
		return err
	}

	cfg, err := config.NewConfig(style)
	if err != nil {
		return err
	}

	logx.Must(pathx.MkdirIfNotExist(dir))
	rootPkg, err := golang.GetParentPackage(dir)
	if err != nil {
		return err
	}

	logx.Must(genEtc(dir, cfg, api))
	logx.Must(genConfig(dir, cfg, api))
	logx.Must(genMain(dir, rootPkg, cfg, api))
	logx.Must(genServiceContext(dir, rootPkg, cfg, api))
	logx.Must(genTypes(dir, cfg, api))
	logx.Must(genRoutes(dir, rootPkg, jwtMiddlewareDir, cfg, api))
	logx.Must(genHandlers(dir, rootPkg, cfg, api))
	logx.Must(genLogic(dir, rootPkg, cfg, api))
	logx.Must(genMiddleware(dir, cfg, api))

	if err := backupAndSweep(apiFile); err != nil {
		return err
	}

	if err := apiformat.ApiFormatByPath(apiFile, false); err != nil {
		return err
	}

	fmt.Println(aurora.Green("Done."))
	return nil
}

func backupAndSweep(apiFile string) error {
	var err error
	var wg sync.WaitGroup

	wg.Add(2)
	_ = os.MkdirAll(tmpDir, os.ModePerm)

	go func() {
		_, fileName := filepath.Split(apiFile)
		_, e := apiutil.Copy(apiFile, fmt.Sprintf(path.Join(tmpDir, tmpFile), fileName, time.Now().Unix()))
		if e != nil {
			err = e
		}
		wg.Done()
	}()
	go func() {
		if e := sweep(); e != nil {
			err = e
		}
		wg.Done()
	}()
	wg.Wait()

	return err
}

func sweep() error {
	keepTime := time.Now().AddDate(0, 0, -7)
	return filepath.Walk(tmpDir, func(fpath string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		pos := strings.LastIndexByte(info.Name(), '-')
		if pos > 0 {
			timestamp := info.Name()[pos+1:]
			seconds, err := strconv.ParseInt(timestamp, 10, 64)
			if err != nil {
				// print error and ignore
				fmt.Println(aurora.Red(fmt.Sprintf("sweep ignored file: %s", fpath)))
				return nil
			}

			tm := time.Unix(seconds, 0)
			if tm.Before(keepTime) {
				if err := os.Remove(fpath); err != nil {
					fmt.Println(aurora.Red(fmt.Sprintf("failed to remove file: %s", fpath)))
					return err
				}
			}
		}

		return nil
	})
}
