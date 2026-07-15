package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Transaction representa uma movimentação vinda da API
type Transaction struct {
	ID       int64   `json:"id"`
	UserID   string  `json:"user_id"`
	Category string  `json:"category"`
	Type     string  `json:"type"`
	Valor    float64 `json:"valor"`
}

// TransactionView é o que o template vai usar para exibir cada linha
type TransactionView struct {
	ID             int64
	Category       string
	Type           string
	Valor          float64
	ValorFormatado string
}

// CategoriaView é cada item da legenda do gráfico
type CategoriaView struct {
	Nome       string
	Valor      float64
	Percentual int
	Cor        string
}

// PageData é o struct que alimenta o template inteiro
type PageData struct {
	MesAno           string
	TotalEntradas    string
	TotalSaidas      string
	Saldo            string
	TotalMovimentado string
	QtdEntradas      int
	QtdSaidas        int
	QtdTotal         int
	Transactions     []TransactionView
	Categorias       []CategoriaView
	DonutGradient    template.CSS
}

// cores para as categorias do gráfico (cicla se houver mais)
var coresCategorias = []string{
	"var(--income)",
	"var(--gold)",
	"var(--expense)",
	"var(--ink-soft)",
	"#6366f1",
	"#06b6d4",
}

// formatarValor formata um float para o padrão brasileiro (ex: 2.500,00)
func formatarValor(v float64) string {
	// Arredonda para 2 casas decimais
	v = math.Round(v*100) / 100

	inteira := int64(v)
	decimal := int64(math.Round((v - float64(inteira)) * 100))
	if decimal < 0 {
		decimal = -decimal
	}

	// Formata a parte inteira com separador de milhar
	s := fmt.Sprintf("%d", inteira)
	if inteira < 0 {
		s = fmt.Sprintf("%d", -inteira)
	}

	// Adiciona pontos como separadores de milhar
	if len(s) > 3 {
		var parts []string
		for len(s) > 3 {
			parts = append([]string{s[len(s)-3:]}, parts...)
			s = s[:len(s)-3]
		}
		parts = append([]string{s}, parts...)
		s = strings.Join(parts, ".")
	}

	return fmt.Sprintf("%s,%02d", s, decimal)
}

// fetchTransactions busca as transações da API
func fetchTransactions() ([]Transaction, error) {
	resp, err := http.Get("https://apifinanceiro-aeeo.onrender.com/api/v1/transactions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API retornou status %d", resp.StatusCode)
	}

	var transactions []Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactions); err != nil {
		return nil, err
	}

	return transactions, nil
}

// buildPageData processa as transações e monta os dados da página
func buildPageData(transactions []Transaction) PageData {
	var totalEntradas, totalSaidas float64
	var qtdEntradas, qtdSaidas int
	categoriasMap := make(map[string]float64)

	for _, t := range transactions {
		if strings.EqualFold(t.Type, "Entrada") {
			totalEntradas += t.Valor
			qtdEntradas++
		} else {
			totalSaidas += t.Valor
			qtdSaidas++
		}
		categoriasMap[t.Category] += t.Valor
	}

	saldo := totalEntradas - totalSaidas
	totalMovimentado := totalEntradas + totalSaidas

	// Montar as views das transações
	var views []TransactionView
	for _, t := range transactions {
		views = append(views, TransactionView{
			ID:             t.ID,
			Category:       t.Category,
			Type:           t.Type,
			Valor:          t.Valor,
			ValorFormatado: formatarValor(t.Valor),
		})
	}

	// Montar categorias ordenadas por valor (decrescente)
	type catEntry struct {
		nome  string
		valor float64
	}
	var catList []catEntry
	for nome, valor := range categoriasMap {
		catList = append(catList, catEntry{nome, valor})
	}
	sort.Slice(catList, func(i, j int) bool {
		return catList[i].valor > catList[j].valor
	})

	var categorias []CategoriaView
	var gradientParts []string
	acumulado := 0.0

	for i, cat := range catList {
		pct := 0
		if totalMovimentado > 0 {
			pct = int(math.Round(cat.valor / totalMovimentado * 100))
		}
		cor := coresCategorias[i%len(coresCategorias)]
		categorias = append(categorias, CategoriaView{
			Nome:       cat.nome,
			Valor:      cat.valor,
			Percentual: pct,
			Cor:        cor,
		})

		inicio := acumulado
		acumulado += float64(pct)
		gradientParts = append(gradientParts, fmt.Sprintf("%s %.0f%% %.0f%%", cor, inicio, acumulado))
	}

	donutGradient := template.CSS(strings.Join(gradientParts, ", "))

	// Mês/Ano atual em português
	meses := []string{"", "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
		"Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro"}
	agora := time.Now()
	mesAno := fmt.Sprintf("%s %d", meses[agora.Month()], agora.Year())

	return PageData{
		MesAno:           mesAno,
		TotalEntradas:    formatarValor(totalEntradas),
		TotalSaidas:      formatarValor(totalSaidas),
		Saldo:            formatarValor(saldo),
		TotalMovimentado: formatarValor(totalMovimentado),
		QtdEntradas:      qtdEntradas,
		QtdSaidas:        qtdSaidas,
		QtdTotal:         len(transactions),
		Transactions:     views,
		Categorias:       categorias,
		DonutGradient:    donutGradient,
	}
}

func main() {
	r := gin.Default()

	// Carrega o template
	tmpl := template.Must(template.ParseFiles("index.html"))

	r.StaticFile("/style.css", "./style.css")

	r.GET("/", func(c *gin.Context) {
		transactions, err := fetchTransactions()
		if err != nil {
			fmt.Printf("Erro ao buscar transações: %v\n", err)
			// Renderiza mesmo sem dados
			transactions = []Transaction{}
		}

		data := buildPageData(transactions)

		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(c.Writer, data); err != nil {
			fmt.Printf("Erro ao renderizar template: %v\n", err)
			c.String(http.StatusInternalServerError, "Erro ao renderizar página")
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Servidor rodando na porta", port)
	r.Run(":" + port)
}
