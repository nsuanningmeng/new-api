package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/availability"
	"github.com/gin-gonic/gin"
)

func GetAvailabilityModels(c *gin.Context) {
	result, err := availability.GetOverview()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

func GetAvailabilityModelGroups(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		common.ApiError(c, errors.New("model parameter required"))
		return
	}
	result, err := availability.GetGroups(modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}
