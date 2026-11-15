// Package apis is app-order's HTTP layer: four hand-written gin handlers,
// none of them a wrapper around core's generic CRUD Actions. See
// service/order.go's package doc for why.
package handler

import (
	"errors"
	"net/http"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/contract/actions"

	// models.Response in the @Success annotations below resolves to
	// motor-core's sdk/contract/models.Response, not to this package -
	// swaggo finds it through --parseDependency. It is the envelope with a
	// data field; core's response.Response, which the framework's own
	// handlers name, has no data field and would document these endpoints
	// as returning none.
	"github.com/gatework/motor/example/app-order/models"
	"github.com/gatework/motor/example/app-order/service"
	orderdto "github.com/gatework/motor/example/app-order/service/dto"
)

// Order embeds api.Api the same way every hand-written motor handler
// does (see app/admin/handler/sys_post.go): MakeContext/MakeOrm/Bind/
// MakeService/OK/Error/PageOK are all core, imported with no dependency on
// motor itself.
type Order struct {
	*api.API
}

// List
// @Summary List orders visible to the caller's data scope
// @Tags order
// @Param status query string false "status"
// @Param orderNo query string false "orderNo"
// @Param pageIndex query int false "pageIndex"
// @Param pageSize query int false "pageSize"
// @Success 200 {object} models.Response
// @Router /api/v1/order [get]
// @Security Bearer
func (e Order) List(c *gin.Context) {
	s := service.Order{}
	req := orderdto.OrderSearchReq{}
	var err error
	e.API, err = newAPI(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.API.Service()
	if err != nil {
		e.Error(http.StatusUnprocessableEntity, err, err.Error())
		return
	}

	p := actions.GetPermissionFromContext(c)
	list := make([]models.Order, 0)
	count, err := s.GetPage(&req, p, &list)
	if err != nil {
		e.Logger.Error(err)
		e.Error(http.StatusInternalServerError, err, "failed to list orders")
		return
	}
	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "ok")
}

// Get
// @Summary Get one order and its items
// @Tags order
// @Param id path int true "order id"
// @Success 200 {object} models.Response
// @Router /api/v1/order/{id} [get]
// @Security Bearer
func (e Order) Get(c *gin.Context) {
	s := service.Order{}
	req := orderdto.OrderIDReq{}
	var err error
	e.API, err = newAPI(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.API.Service()
	if err != nil {
		e.Error(http.StatusUnprocessableEntity, err, err.Error())
		return
	}

	p := actions.GetPermissionFromContext(c)
	var order models.Order
	if err = s.Get(req.ID, p, &order); err != nil {
		e.Error(http.StatusNotFound, err, "order not found")
		return
	}
	e.OK(order, "ok")
}

// Create
// @Summary Place a new order
// @Tags order
// @Accept application/json
// @Param data body orderdto.OrderCreateReq true "data"
// @Success 200 {object} models.Response
// @Router /api/v1/order [post]
// @Security Bearer
func (e Order) Create(c *gin.Context) {
	s := service.Order{}
	req := orderdto.OrderCreateReq{}
	var err error
	e.API, err = newAPI(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.API.Service()
	if err != nil {
		e.Error(http.StatusUnprocessableEntity, err, err.Error())
		return
	}

	order, err := s.Create(&req, user.ID(c))
	if err != nil {
		if errors.Is(err, service.OrderEmptyErr) {
			e.Error(http.StatusBadRequest, err, err.Error())
			return
		}
		e.Logger.Error(err)
		e.Error(http.StatusInternalServerError, err, "failed to create order")
		return
	}
	e.OK(order, "created")
}

// Pay
// @Summary Mark a pending order as paid
// @Tags order
// @Param id path int true "order id"
// @Success 200 {object} models.Response
// @Router /api/v1/order/{id}/pay [put]
// @Security Bearer
func (e Order) Pay(c *gin.Context) {
	s := service.Order{}
	req := orderdto.OrderIDReq{}
	var err error
	e.API, err = newAPI(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.API.Service()
	if err != nil {
		e.Error(http.StatusUnprocessableEntity, err, err.Error())
		return
	}

	p := actions.GetPermissionFromContext(c)
	if err = s.Pay(req.ID, p); err != nil {
		if errors.Is(err, service.OrderNotPendingErr) {
			// Deliberately the same response whether the order does not
			// exist, is already paid, or is outside p's data scope - see
			// service.Order.Pay's doc comment.
			e.Error(http.StatusConflict, err, err.Error())
			return
		}
		e.Logger.Error(err)
		e.Error(http.StatusInternalServerError, err, "payment failed")
		return
	}
	e.OK(nil, "paid")
}

func newAPI(c *gin.Context) (*api.API, error) {
	db, err := pkg.GetORM(c)
	a, constructionErr := api.New(c, api.Dependencies{DB: db, Logger: sdk.Runtime.Logger(), Cache: sdk.Runtime.Cache()})
	if constructionErr != nil {
		return nil, constructionErr
	}
	return a, err
}
