# CHANGELOG

## 0.3.0

- Добавлена загрузка локального `endge-knowledge/v1` bundle и детерминированный поиск по документации.
- Добавлена включаемая явно диагностика retrieval в Git-ignored каталоге `tmp/debug`.
- Добавлен детерминированный отбор документов из `ExportLive` и последних сообщений диалога для будущего model context.

## 0.1.0

- Создан базовый Go-сервис `github.com/endge-lab/service-ai-workbench`.
- Подключены config, HTTP, logging, telemetry и optional platform integrations из `service-kit-go`.
- Добавлены технические endpoints `/health`, `/version` и non-production OpenAPI.
- Подготовлены локальный запус на порту `8081` и Docker scaffold.
