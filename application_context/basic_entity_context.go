package application_context

import "mahresources/server/interfaces"

type EntityWriter[T interfaces.BasicEntityReader] struct {
	ctx *MahresourcesContext
}

func NewEntityWriter[T interfaces.BasicEntityReader](ctx *MahresourcesContext) *EntityWriter[T] {
	return &EntityWriter[T]{ctx: ctx}
}

func (w *EntityWriter[T]) UpdateName(id uint, name string) error {
	entity := new(T)
	return w.ctx.db.Model(entity).Where("id = ?", id).Update("name", name).Error
}

func (w *EntityWriter[T]) UpdateDescription(id uint, description string) error {
	entity := new(T)
	return w.ctx.db.Model(entity).Where("id = ?", id).Update("description", description).Error
}
