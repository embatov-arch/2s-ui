package cronjob

import (
	"context"
	"time"

	"github.com/shenaba/2s-ui/logger"
	"github.com/shenaba/2s-ui/service"
)

type PackageExcludeJob struct{}

func NewPackageExcludeJob() *PackageExcludeJob { return &PackageExcludeJob{} }

func (j *PackageExcludeJob) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := service.PackageExcludeRefreshIfDue(ctx); err != nil {
		logger.Warning("package exclusion refresh failed, retaining cached lists: ", err)
	}
}
