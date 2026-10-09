package archtest

import (
	"testing"

	"git.stit.tech/stit-core/golibinfra/archkit"
)

// TestLayout: раскладку сервиса держит archkit — правила приходят версией golibinfra (перечень и тексты
// отказов — README кита). Настройки сервиса — layout в layout_settings_test.go: это файл форка, перенос
// обвязки его не трогает, а без него пакет не соберётся.
func TestLayout(t *testing.T) {
	t.Parallel()
	archkit.Check(t, layout...)
}
