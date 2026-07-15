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
	"github.com/gorilla/sessions"
)

// Store de sessão (cookie criptografado)
var store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_SECRET")))

// ── Structs para transações ──

type Transaction struct {
	ID       int64   `json:"id"`
	UserID   string  `json:"user_id"`
	Category string  `json:"category"`
	Type     string  `json:"type"`
	Valor    float64 `json:"valor"`
}

type TransactionView struct {
	ID             int64
	Category       string
	Type           string
	Valor          float64
	ValorFormatado string
}

type CategoriaView struct {
	Nome       string
	Valor      float64
	Percentual int
	Cor        string
}

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

// Cores para as categorias do gráfico
var coresCategorias = []string{
	"var(--income)",
	"var(--gold)",
	"var(--expense)",
	"var(--ink-soft)",
	"#6366f1",
	"#06b6d4",
}

// ── Funções auxiliares ──

func formatarValor(v float64) string {
	v = math.Round(v*100) / 100
	inteira := int64(v)
	decimal := int64(math.Round((v - float64(inteira)) * 100))
	if decimal < 0 {
		decimal = -decimal
	}

	s := fmt.Sprintf("%d", inteira)
	if inteira < 0 {
		s = fmt.Sprintf("%d", -inteira)
	}

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

func buildPageData(transactions []Transaction) PageData {
	var totalEntradas, totalSaidas float64
	var qtdEntradas, qtdSaidas int
	categoriaSaidasMap := make(map[string]float64) // Só saídas para o gráfico

	for _, t := range transactions {
		if strings.EqualFold(t.Type, "Entrada") {
			totalEntradas += t.Valor
			qtdEntradas++
		} else {
			totalSaidas += t.Valor
			qtdSaidas++
			categoriaSaidasMap[t.Category] += t.Valor
		}
	}

	saldo := totalEntradas - totalSaidas
	totalMovimentado := totalEntradas + totalSaidas

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

	type catEntry struct {
		nome  string
		valor float64
	}
	var catList []catEntry
	for nome, valor := range categoriaSaidasMap {
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
		if totalSaidas > 0 {
			pct = int(math.Round(cat.valor / totalSaidas * 100))
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

// ── Servidor principal ──

func main() {
	r := gin.Default()

	// Carrega o template da página principal
	mainTmpl := template.Must(template.ParseFiles("main/index.html"))

	// Arquivos estáticos
	r.StaticFile("/style.css", "./style.css")
	r.StaticFile("/main/style.css", "./main/style.css")

	// ── Página de login (raiz) ──
	r.GET("/", func(c *gin.Context) {
		c.File("index.html")
	})

	// ── Rota POST do login ──
	r.POST("/login", func(c *gin.Context) {
		usuario := c.PostForm("username")
		senha := c.PostForm("password")

		fmt.Printf("Tentativa de login → usuário: %s\n", usuario)

		resp, err := http.Get("https://apifinanceiro-aeeo.onrender.com/api/v1/users")
		if err != nil {
			c.String(http.StatusInternalServerError, "Erro ao conectar com a API.")
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			c.String(http.StatusInternalServerError, "Erro na resposta da API.")
			return
		}

		var users []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
			c.String(http.StatusInternalServerError, "Erro ao decodificar usuários.")
			return
		}

		var loggedUserID string
		var loggedUserName string
		userValid := false
		for _, u := range users {
			if u.Name == usuario && u.Password == senha {
				userValid = true
				loggedUserID = u.ID
				loggedUserName = u.Name
				break
			}
		}

		if userValid {
			// Salva o user_id e nome na sessão
			session, _ := store.Get(c.Request, "livro-session")
			session.Values["user_id"] = loggedUserID
			session.Values["user_name"] = loggedUserName
			session.Save(c.Request, c.Writer)

			c.Header("HX-Redirect", "/dashboard")
			c.String(http.StatusOK, "")
			return
		}

		c.String(http.StatusUnauthorized, "Usuário inválido ou não encontrado.")
	})

	// ── Página principal (dashboard) com Go template ──
	r.GET("/dashboard", func(c *gin.Context) {
		// Verifica se o usuário está logado
		session, _ := store.Get(c.Request, "livro-session")
		userID, ok := session.Values["user_id"].(string)
		if !ok || userID == "" {
			c.Redirect(http.StatusFound, "/")
			return
		}

		transactions, err := fetchTransactions()
		if err != nil {
			fmt.Printf("Erro ao buscar transações: %v\n", err)
			transactions = []Transaction{}
		}

		// Filtra apenas as transações do usuário logado
		var userTransactions []Transaction
		for _, t := range transactions {
			if t.UserID == userID {
				userTransactions = append(userTransactions, t)
			}
		}

		data := buildPageData(userTransactions)

		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := mainTmpl.Execute(c.Writer, data); err != nil {
			fmt.Printf("Erro ao renderizar template: %v\n", err)
			c.String(http.StatusInternalServerError, "Erro ao renderizar página")
		}
	})

	// ── Criar movimentação (formulário do dashboard) ──
	r.POST("/transaction", func(c *gin.Context) {
		// Verifica se o usuário está logado
		session, _ := store.Get(c.Request, "livro-session")
		userID, ok := session.Values["user_id"].(string)
		if !ok || userID == "" {
			c.String(http.StatusUnauthorized, `<span style="color:var(--expense)">Sessão expirada. Faça login novamente.</span>`)
			return
		}

		valorStr := c.PostForm("valor")
		category := c.PostForm("category")
		tipo := c.PostForm("type")

		// Converte o valor para float
		var valor float64
		fmt.Sscanf(valorStr, "%f", &valor)

		if valor <= 0 || category == "" || tipo == "" {
			c.String(http.StatusBadRequest, `<span style="color:var(--expense)">Preencha todos os campos corretamente.</span>`)
			return
		}

		// Monta o JSON usando o user_id da sessão
		payload := map[string]interface{}{
			"user_id":  userID,
			"category": category,
			"type":     tipo,
			"valor":    valor,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			c.String(http.StatusInternalServerError, `<span style="color:var(--expense)">Erro ao preparar dados.</span>`)
			return
		}

		resp, err := http.Post(
			"https://apifinanceiro-aeeo.onrender.com/api/v1/transactions",
			"application/json",
			strings.NewReader(string(jsonData)),
		)
		if err != nil {
			c.String(http.StatusInternalServerError, `<span style="color:var(--expense)">Erro ao conectar com a API.</span>`)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
			// Redireciona de volta ao dashboard para atualizar os dados
			c.Header("HX-Redirect", "/dashboard")
			c.String(http.StatusOK, "")
			return
		}

		c.String(http.StatusInternalServerError, `<span style="color:var(--expense)">Erro ao salvar movimentação.</span>`)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Servidor rodando na porta", port)
	r.Run(":" + port)
}
