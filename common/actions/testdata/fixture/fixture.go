// Package fixture holds the models and requests the compile test pairs, so
// that common/actions' tests depend on nothing outside common.
package fixture

import "github.com/gin-gonic/gin"

// Widget is a model the actions accept.
type Widget struct{ ID int }

func (*Widget) TableName() string { return "widget" }
func (*Widget) SetCreateBy(int)   {}
func (*Widget) SetUpdateBy(int)   {}
func (w *Widget) GetID() any      { return w.ID }

// Gadget is another model the actions accept.
type Gadget struct{ ID int }

func (*Gadget) TableName() string { return "gadget" }
func (*Gadget) SetCreateBy(int)   {}
func (*Gadget) SetUpdateBy(int)   {}
func (g *Gadget) GetID() any      { return g.ID }

// WidgetControl builds a Widget.
type WidgetControl struct{}

func (*WidgetControl) Bind(*gin.Context) error   { return nil }
func (*WidgetControl) ToModel() (*Widget, error) { return &Widget{}, nil }

// GadgetControl builds a Gadget.
type GadgetControl struct{}

func (*GadgetControl) Bind(*gin.Context) error   { return nil }
func (*GadgetControl) ToModel() (*Gadget, error) { return &Gadget{}, nil }
