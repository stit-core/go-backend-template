// Package paging — страница списка: один тип на вход list-операции huma (встраивается в её
// input), usecase и SQL.
package paging

const (
	DefaultSize = 20
	MaxSize     = 100
)

// Page — номер страницы с 1 и её размер; теги — query-параметры huma с теми же дефолтом и пределом.
type Page struct {
	Number int `default:"1"  minimum:"1"   query:"page"`
	Size   int `default:"20" maximum:"100" minimum:"1"  query:"size"`
}

// Normalize приводит страницу к допустимой: номер с 1, размер по умолчанию и не больше MaxSize.
func (p Page) Normalize() Page {
	if p.Number < 1 {
		p.Number = 1
	}
	if p.Size <= 0 {
		p.Size = DefaultSize
	}
	if p.Size > MaxSize {
		p.Size = MaxSize
	}
	return p
}

// Offset — сколько записей до страницы.
func (p Page) Offset() int { return (p.Number - 1) * p.Size }
