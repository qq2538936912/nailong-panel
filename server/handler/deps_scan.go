package handler

import (
	"panel/config"
	"panel/pkg/response"
	"panel/service"

	"github.com/gin-gonic/gin"
)

func (h *DepsHandler) ScanMissing(c *gin.Context) {
	result, err := service.ScanMissingDependencies(config.C.Data.ScriptsDir)
	if err != nil {
		response.InternalError(c, "扫描脚本依赖失败")
		return
	}

	response.Success(c, gin.H{"data": result})
}
