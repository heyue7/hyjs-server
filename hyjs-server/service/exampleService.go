package service

import (
	"hyjs-server/db"
	"hyjs-server/model"

	"github.com/gin-gonic/gin"
)

func ExampleService(c *gin.Context, req model.ExampleListRequest) (resp model.ExampleListResponse, err error) {
	var lastName string
	if lastName, err = exampleFunc(c, req.FirstName); err != nil {
		return
	}

	var exampleModel db.ExampleModel
	if err = db.GetIns().WithContext(c).Where("first_name = ?", req.FirstName).First(&exampleModel).Error; err != nil {
		return
	}

	resp.LastName = lastName
	resp = model.ExampleListResponse{
		LastName: exampleModel.LastName,
	}

	return
}

func exampleFunc(c *gin.Context, firstName string) (lastName string, err error) {
	return
}

func example2Func(c *gin.Context, firstName string) (string, error) {
	return "lastName", nil
}
