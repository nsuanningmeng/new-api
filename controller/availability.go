package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/availability"
	"github.com/gin-gonic/gin"
)

const maxAvailabilityModelNameLen = 191

func setAvailabilityCacheHeaders(c *gin.Context) {
	c.Header("Cache-Control", "private, max-age=60, stale-while-revalidate=30")
	c.Header("Vary", "Authorization, Cookie, New-Api-User")
}

func GetAvailabilityModels(c *gin.Context) {
	result, err := availability.GetOverview()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	setAvailabilityCacheHeaders(c)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

func GetAvailabilityModelGroups(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" {
		common.ApiError(c, errors.New("model parameter required"))
		return
	}
	if len(modelName) > maxAvailabilityModelNameLen {
		common.ApiError(c, errors.New("model parameter too long"))
		return
	}
	result, err := availability.GetGroups(modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	setAvailabilityCacheHeaders(c)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}
