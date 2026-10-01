// Package paging — страница списка: один тип на вход list-операции huma (встраивается в её
// input), usecase и SQL.
package paging

// MaxNumber держит смещение (MaxNumber-1)*MaxSize в int4 запроса: глубже offset-страницы не листают.
const (
	DefaultSize = 20
	MaxSize     = 100
	MaxNumber   = 100000
)

// Page — номер страницы с 1 и её размер; теги — query-параметры huma с теми же дефолтами и пределами.
type Page struct {
	Number int `default:"1"  maximum:"100000" minimum:"1" query:"page"`
	Size   int `default:"20" maximum:"100"    minimum:"1" query:"size"`
}

// Normalize приводит страницу к допустимой: номер от 1 до MaxNumber, размер по умолчанию и не больше
// MaxSize.
func (p Page) Normalize() Page {
	if p.Number < 1 {
		p.Number = 1
	}
	if p.Number > MaxNumber {
		p.Number = MaxNumber
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
