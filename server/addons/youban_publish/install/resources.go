package install

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gres"

	"hotgo/internal/library/addons"
)

const publishModuleName = "youban_publish"

var publishDefaultStaticFiles = []string{
	"antiscan/default-preview.webp",
}

func syncStaticResources(ctx context.Context) error {
	for _, relPath := range publishDefaultStaticFiles {
		if err := syncStaticResource(ctx, relPath); err != nil {
			return err
		}
	}
	return nil
}

func syncStaticResource(ctx context.Context, relPath string) error {
	dst := gfile.Join(addons.GetResourcePath(ctx), "addons", publishModuleName, "public", relPath)
	if gfile.Exists(dst) && gfile.Size(dst) > 0 {
		return nil
	}
	if err := gfile.Mkdir(gfile.Dir(dst)); err != nil {
		return gerror.Wrapf(err, "创建静态资源目录失败：%s", gfile.Dir(dst))
	}

	sourcePaths := []string{
		gfile.Join("server", "resource", "addons", publishModuleName, "public", relPath),
		gfile.Join("resource", "addons", publishModuleName, "public", relPath),
	}
	for _, src := range sourcePaths {
		if gfile.Exists(src) {
			if err := gfile.CopyFile(src, dst); err != nil {
				return gerror.Wrapf(err, "复制静态资源失败：%s", relPath)
			}
			return nil
		}
	}

	resourcePath := gfile.Join("resource", "addons", publishModuleName, "public", relPath)
	if !gres.IsEmpty() && gres.Contains(resourcePath) {
		if err := gfile.PutBytes(dst, gres.GetContent(resourcePath)); err != nil {
			return gerror.Wrapf(err, "写入静态资源失败：%s", relPath)
		}
		return nil
	}

	return gerror.Newf("默认静态资源不存在：%s", relPath)
}
