package dbinit

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gres"
	"github.com/gogf/gf/v2/text/gstr"
)

func HasTable(ctx context.Context, table string) (bool, error) {
	tables, err := g.DB().Tables(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range tables {
		if strings.EqualFold(item, table) {
			return true, nil
		}
	}
	return false, nil
}

func ImportFile(ctx context.Context, path string) error {
	content := readSQLFile(path)
	if strings.TrimSpace(content) == "" {
		return gerror.Newf("SQL初始化文件不存在或为空：%s", path)
	}
	for index, stmt := range SplitSQL(content) {
		if shouldSkipStatement(stmt) {
			continue
		}
		if _, err := g.DB().Exec(ctx, stmt); err != nil {
			return gerror.Wrapf(err, "执行 SQL 失败：%s 第 %d 段", path, index+1)
		}
	}
	return nil
}

func readSQLFile(path string) string {
	if gfile.Exists(path) && gfile.IsFile(path) {
		return gfile.GetContents(path)
	}
	if !gres.IsEmpty() && gres.Contains(path) {
		return string(gres.GetContent(path))
	}
	return ""
}

func SplitSQL(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	var statements []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	inLineComment := false
	inBlockComment := false
	for i := 0; i < len(content); i++ {
		ch := content[i]
		next := byte(0)
		if i+1 < len(content) {
			next = content[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if !inSingle && !inDouble && ch == '-' && next == '-' {
			inLineComment = true
			i++
			continue
		}
		if !inSingle && !inDouble && ch == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
		} else if ch == '"' && !inSingle {
			inDouble = !inDouble
		}
		if ch == ';' && !inSingle && !inDouble {
			appendStatement(&statements, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(ch)
	}
	appendStatement(&statements, b.String())
	return statements
}

func appendStatement(statements *[]string, stmt string) {
	stmt = gstr.Trim(stmt)
	if stmt == "" {
		return
	}
	*statements = append(*statements, stmt)
}

func shouldSkipStatement(stmt string) bool {
	upper := strings.ToUpper(strings.TrimSpace(stmt))
	if upper == "" {
		return true
	}
	return strings.HasPrefix(upper, "(")
}
