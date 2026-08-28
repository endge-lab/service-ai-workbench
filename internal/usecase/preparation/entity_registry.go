package preparation

import (
	"slices"
	"strings"
)

type entityDefinition struct {
	collection string
	aliases    []string
	prefixes   []string
}

var entityRegistry = []entityDefinition{
	{collection: "projects", aliases: []string{"проект", "проекты", "project", "projects"}, prefixes: []string{"проект"}},
	{collection: "tenants", aliases: []string{"тенант", "тенанты", "tenant", "tenants"}, prefixes: []string{"тенант"}},
	{collection: "environments", aliases: []string{"окружение", "окружения", "environment", "environments"}, prefixes: []string{"окружен"}},
	{collection: "folders", aliases: []string{"папка", "папки", "раздел", "разделы", "folder", "folders"}, prefixes: []string{"папк", "раздел"}},
	{collection: "types", aliases: []string{"тип", "типы", "type", "types"}, prefixes: []string{"тип"}},
	{collection: "queries", aliases: []string{"запрос", "запросы", "query", "queries"}, prefixes: []string{"запрос"}},
	{collection: "data-views", aliases: []string{"представление", "представления", "data view", "data views"}, prefixes: []string{"представлен"}},
	{collection: "compositions", aliases: []string{"композиция", "композиции", "composition", "compositions"}, prefixes: []string{"композиц"}},
	{collection: "stores", aliases: []string{"хранилище", "хранилища", "store", "stores"}, prefixes: []string{"хранилищ"}},
	{collection: "streams", aliases: []string{"поток", "потоки", "stream", "streams"}, prefixes: []string{"поток"}},
	{collection: "updates", aliases: []string{"обновление", "обновления", "update", "updates"}, prefixes: []string{"обновлен"}},
	{collection: "mocks", aliases: []string{"мок", "моки", "тестовые данные", "mock", "mocks"}, prefixes: []string{"мок"}},
	{collection: "components", aliases: []string{"компонент", "компоненты", "component", "components"}, prefixes: []string{"компонент"}},
	{collection: "actions", aliases: []string{"действие", "действия", "action", "actions"}, prefixes: []string{"действ"}},
	{collection: "filters", aliases: []string{"фильтр", "фильтры", "filter", "filters"}, prefixes: []string{"фильтр"}},
	{collection: "converters", aliases: []string{"конвертер", "конвертеры", "converter", "converters"}, prefixes: []string{"конвертер"}},
	{collection: "computations", aliases: []string{"вычисление", "вычисления", "computation", "computations"}, prefixes: []string{"вычислен"}},
	{collection: "vocabs", aliases: []string{"словарь", "словари", "vocab", "vocabs"}, prefixes: []string{"словар"}},
	{collection: "i18n-bundles", aliases: []string{"словарь переводов", "словари переводов", "translation bundle", "translation bundles"}, prefixes: []string{"перевод"}},
	{collection: "auth-profiles", aliases: []string{"профиль аутентификации", "профили аутентификации", "auth profile", "auth profiles"}, prefixes: []string{"аутентификац"}},
	{collection: "navigations", aliases: []string{"навигация", "navigation"}, prefixes: []string{"навигац"}},
	{collection: "styles", aliases: []string{"стиль", "стили", "style", "styles"}, prefixes: []string{"стил"}},
	{collection: "configurations", aliases: []string{"конфигурация", "конфигурации", "configuration", "configurations"}, prefixes: []string{"конфигурац"}},
}

func expectedEntityTypes(text string) []string {
	tokens := lexicalTokens(normalizeText(text))
	result := make([]string, 0, 2)
	for _, definition := range entityRegistry {
		for _, alias := range definition.aliases {
			if containsTokenSequence(tokens, strings.Fields(alias)) {
				result = append(result, definition.collection)
				break
			}
		}
		if slices.Contains(result, definition.collection) {
			continue
		}
		for _, token := range tokens {
			if hasAnyPrefix(token, definition.prefixes) {
				result = append(result, definition.collection)
				break
			}
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func entityAliasTokens(collections []string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, definition := range entityRegistry {
		if len(collections) > 0 && !slices.Contains(collections, definition.collection) {
			continue
		}
		for _, alias := range definition.aliases {
			for _, token := range strings.Fields(alias) {
				result[token] = struct{}{}
			}
		}
	}
	return result
}

func knownEntityType(value string) bool {
	for _, definition := range entityRegistry {
		if definition.collection == value {
			return true
		}
	}
	return false
}

func isEntityAliasToken(token string, collections []string) bool {
	for _, definition := range entityRegistry {
		if len(collections) > 0 && !slices.Contains(collections, definition.collection) {
			continue
		}
		if hasAnyPrefix(token, definition.prefixes) {
			return true
		}
		for _, alias := range definition.aliases {
			if !strings.Contains(alias, " ") && token == alias {
				return true
			}
		}
	}
	return false
}

func hasAnyPrefix(token string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}
