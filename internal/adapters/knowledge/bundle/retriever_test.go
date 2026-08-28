package bundle

import "testing"

func TestRussianMorphologyProducesStableSearchStems(t *testing.T) {
	if stemToken("фильтров") != stemToken("фильтр") {
		t.Fatalf("filter forms have different stems: %q %q", stemToken("фильтров"), stemToken("фильтр"))
	}
	if stemToken("композиции") != stemToken("композиция") {
		t.Fatalf("composition forms have different stems: %q %q", stemToken("композиции"), stemToken("композиция"))
	}
}

func TestDocumentationRankingUsesHeadingAndMorphology(t *testing.T) {
	query := buildQuery("Объясни синтаксис фильтров")
	relevant := chunk{Title: "Фильтры", Heading: "Синтаксис фильтра", searchText: normalize("Фильтр задает условия выборки"), searchTokens: tokenSet(normalize("Фильтр задает условия выборки"))}
	unrelated := chunk{Title: "Общий синтаксис", Heading: "Значения", searchText: normalize("Значения и типы"), searchTokens: tokenSet(normalize("Значения и типы"))}
	relevantScore, _ := scoreChunk(relevant, query)
	unrelatedScore, _ := scoreChunk(unrelated, query)
	if relevantScore <= unrelatedScore || relevantScore < 20 {
		t.Fatalf("relevant filter documentation was not ranked first: relevant=%d unrelated=%d query=%#v", relevantScore, unrelatedScore, query)
	}
}

func TestDocumentationTokensStripCodePunctuation(t *testing.T) {
	query := buildQuery("Объясни, как работает defineComposition и для чего он нужен.")
	if !contains(query.Terms, "definecomposition") || contains(query.Terms, "нужен.") {
		t.Fatalf("code symbol query was tokenized incorrectly: %#v", query)
	}
	candidate := chunk{
		Title:        "Composition",
		Heading:      "Composition",
		searchText:   normalize("defineComposition({ activateOn: startup() })"),
		searchTokens: tokenSet(normalize("defineComposition({ activateOn: startup() })")),
	}
	score, matched := scoreChunk(candidate, query)
	if score < 120 || !contains(matched, "definecomposition") {
		t.Fatalf("code symbol was not matched: score=%d matched=%v query=%#v", score, matched, query)
	}
	incidental := chunk{
		Title:        "Select",
		Heading:      "Select",
		searchText:   normalize("defineComposition({ runtimes: {} })"),
		searchTokens: tokenSet(normalize("defineComposition({ runtimes: {} })")),
	}
	incidentalScore, _ := scoreChunk(incidental, query)
	if score <= incidentalScore {
		t.Fatalf("concept overview must outrank incidental code example: overview=%d incidental=%d", score, incidentalScore)
	}
}

func TestTechnicalAPIAnchorOutranksGenericDocumentationTerms(t *testing.T) {
	query := buildQuery("Кратко объясни назначение filterView по документации Endge")
	reference := chunk{
		Title:        "Filter",
		Heading:      "FilterView в Composition",
		searchText:   normalize("filterView выбирает поля и controls одного Filter runtime"),
		searchTokens: tokenSet(normalize("filterView выбирает поля и controls одного Filter runtime")),
	}
	generic := chunk{
		Title:        "Документация Endge",
		Heading:      "Назначение документации",
		searchText:   normalize("Документация Endge содержит примеры использования API"),
		searchTokens: tokenSet(normalize("Документация Endge содержит примеры использования API")),
	}
	referenceScore, _ := scoreChunk(reference, query)
	genericScore, _ := scoreChunk(generic, query)
	if referenceScore <= genericScore || referenceScore < 35 {
		t.Fatalf("exact API anchor must dominate generic prose: reference=%d generic=%d", referenceScore, genericScore)
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
