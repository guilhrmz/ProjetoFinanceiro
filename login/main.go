package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqRequest struct {
	Model    string        `json:"model"`
	Messages []groqMessage `json:"messages"`
}

type groqResponse struct {
	Choices []struct {
		Message groqMessage `json:"message"`
	} `json:"choices"`
}

func modalErro(msg string) string {
	return fmt.Sprintf(`
		<div style="position:fixed;inset:0;background:rgba(0,0,0,0.35);z-index:1000;display:flex;align-items:center;justify-content:center;">
			<div style="background:#fff;border:1px solid #E5E7EB;border-radius:10px;padding:22px;width:100%%;max-width:420px;display:flex;flex-direction:column;gap:14px;box-shadow:0 4px 20px rgba(0,0,0,0.08);">
				<h3 style="margin:0;font-family:'Inter',sans-serif;font-size:0.7rem;font-weight:600;color:#DC2626;text-transform:uppercase;letter-spacing:0.1em;">Atenção</h3>
				<p style="margin:0;font-family:'Inter',sans-serif;font-size:0.875rem;color:#111827;">%s</p>
				<div style="display:flex;justify-content:flex-end;">
					<button type="button" hx-get="/modal-fechar" hx-target="#modal-container" hx-swap="innerHTML"
						style="padding:7px 16px;border-radius:6px;background:#111827;color:#fff;border:none;cursor:pointer;font-family:'Inter',sans-serif;font-size:0.875rem;font-weight:500;">Fechar</button>
				</div>
			</div>
		</div>
	`, msg)
}

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
	MesAno        string
	TotalEntradas string
	TotalSaidas   string
	Saldo         string
	QtdEntradas   int
	QtdSaidas     int
	QtdTotal      int
	Transactions  []TransactionView
	Categorias    []CategoriaView
	PieChart      template.HTML
}

var coresCategorias = []string{
	"#059669",
	"#2563EB",
	"#D97706",
	"#7C3AED",
	"#0891B2",
	"#DC2626",
}

func gerarPieChart(categorias []CategoriaView, total float64) template.HTML {
	if total == 0 || len(categorias) == 0 {
		return template.HTML(`<svg viewBox="0 0 160 160" width="160" height="160"><circle cx="80" cy="80" r="70" fill="#F3F4F6"/></svg>`)
	}

	cx, cy, r := 80.0, 80.0, 70.0
	angle := -math.Pi / 2
	paths := ""

	for _, cat := range categorias {
		if cat.Valor <= 0 {
			continue
		}
		sweep := (cat.Valor / total) * 2 * math.Pi
		endAngle := angle + sweep

		x1 := cx + r*math.Cos(angle)
		y1 := cy + r*math.Sin(angle)
		x2 := cx + r*math.Cos(endAngle)
		y2 := cy + r*math.Sin(endAngle)

		largeArc := 0
		if sweep > math.Pi {
			largeArc = 1
		}

		paths += fmt.Sprintf(
			`<path d="M %.4f %.4f L %.4f %.4f A %.4f %.4f 0 %d 1 %.4f %.4f Z" fill="%s" stroke="#fff" stroke-width="2"/>`,
			cx, cy, x1, y1, r, r, largeArc, x2, y2, cat.Cor,
		)
		angle = endAngle
	}

	return template.HTML(fmt.Sprintf(`<svg viewBox="0 0 160 160" width="160" height="160">%s</svg>`, paths))
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
	}

	pieChart := gerarPieChart(categorias, totalSaidas)

	meses := []string{"", "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
		"Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro"}
	agora := time.Now()
	mesAno := fmt.Sprintf("%s %d", meses[agora.Month()], agora.Year())

	return PageData{
		MesAno:        mesAno,
		TotalEntradas: formatarValor(totalEntradas),
		TotalSaidas:   formatarValor(totalSaidas),
		Saldo:         formatarValor(saldo),
		QtdEntradas:   qtdEntradas,
		QtdSaidas:     qtdSaidas,
		QtdTotal:      len(transactions),
		Transactions:  views,
		Categorias:    categorias,
		PieChart:      pieChart,
	}
}

// ── Servidor principal ──

func main() {
	r := gin.Default()

	// Carrega o template da página principal
	mainTmpl := template.Must(template.ParseFiles("main/index.html"))

	// Arquivos estáticos
	r.StaticFile("/style.css", "./login/style.css")
	r.StaticFile("/main/style.css", "./main/style.css")

	// ── Página de login (raiz) ──
	r.GET("/", func(c *gin.Context) {
		c.File("login/index.html")
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
		c.Header("Cache-Control", "no-store")
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

	r.GET("/modal-ia", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, `
			<div style="position:fixed;inset:0;background:rgba(0,0,0,0.35);z-index:1000;display:flex;align-items:center;justify-content:center;">
				<div style="background:#fff;border:1px solid #E5E7EB;border-radius:10px;padding:22px;width:100%;max-width:420px;display:flex;flex-direction:column;gap:14px;box-shadow:0 4px 20px rgba(0,0,0,0.08);">
					<h3 style="margin:0;font-family:'Inter',sans-serif;font-size:0.7rem;font-weight:600;color:#6B7280;text-transform:uppercase;letter-spacing:0.1em;">Adicionar com IA</h3>
					<form hx-post="/ia" hx-target="#modal-container" hx-swap="innerHTML">
						<textarea name="teste" rows="4" placeholder="Ex: gastei 50 reais no mercado, recebi 3000 de salário..." style="width:100%;resize:none;border-radius:6px;border:1px solid #E5E7EB;padding:10px 12px;font-size:0.875rem;background:#F9FAFB;color:#111827;font-family:'Inter',sans-serif;outline:none;transition:border-color 0.15s;"></textarea>
						<div style="display:flex;gap:8px;justify-content:flex-end;margin-top:12px;">
							<button type="button" hx-get="/modal-fechar" hx-target="#modal-container" hx-swap="innerHTML"
								style="padding:7px 16px;border-radius:6px;border:1px solid #E5E7EB;background:#fff;color:#6B7280;cursor:pointer;font-family:'Inter',sans-serif;font-size:0.875rem;font-weight:500;">Cancelar</button>
							<button type="submit" style="padding:7px 16px;border-radius:6px;background:#111827;color:#fff;border:none;cursor:pointer;font-family:'Inter',sans-serif;font-size:0.875rem;font-weight:500;">Confirmar</button>
						</div>
					</form>
				</div>
			</div>
		`)
	})

	r.GET("/modal-fechar", func(c *gin.Context) {
		c.String(http.StatusOK, "")
	})

	r.POST("/ia", func(c *gin.Context) {
		session, _ := store.Get(c.Request, "livro-session")
		userID, ok := session.Values["user_id"].(string)
		if !ok || userID == "" {
			c.String(http.StatusOK, modalErro("Sessão expirada. Faça login novamente."))
			return
		}

		texto := c.PostForm("teste")
		fmt.Println("teste:", texto)

		systemPrompt := `Você é um classificador de movimentações financeiras. Sua única função é identificar transações financeiras explícitas no texto.

CRITÉRIOS OBRIGATÓRIOS para considerar uma movimentação válida:
- O texto deve mencionar explicitamente um valor numérico (ex: 50 reais, R$100, 200)
- E deve indicar claramente uma transação (gastei, paguei, recebi, comprei, salário, aluguel, etc.)
- Ambos precisam estar presentes. Sem valor numérico = irrelevante. Sem ação financeira = irrelevante.

Se o texto for uma saudação, pergunta, frase aleatória, ou qualquer coisa que não seja uma transação financeira com valor explícito, retorne EXATAMENTE: {"error":"irrelevante"}

Se for válido, retorne SOMENTE um array JSON puro, sem markdown, sem explicações:
[{"user_id":"` + userID + `","category":"CATEGORIA","type":"Entrada ou Saida","valor":0.00}]

Categorias disponíveis: Alimentação, Transporte, Moradia, Salário, Lazer, Outros
type: "Entrada" para receitas/salário, "Saida" para gastos/despesas
user_id: sempre "` + userID + `"
Não retorne nada além do JSON.`

		reqBody, _ := json.Marshal(groqRequest{
			Model: "llama-3.1-8b-instant",
			Messages: []groqMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: texto},
			},
		})

		httpReq, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("GROQ_API_KEY"))
		httpReq.Header.Set("Content-Type", "application/json")

		httpResp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			c.String(http.StatusOK, modalErro("Erro ao conectar com a IA. Tente novamente."))
			return
		}
		defer httpResp.Body.Close()

		body, _ := io.ReadAll(httpResp.Body)
		var gr groqResponse
		if err := json.Unmarshal(body, &gr); err != nil || len(gr.Choices) == 0 {
			c.String(http.StatusOK, modalErro("Erro ao processar resposta da IA."))
			return
		}

		content := strings.TrimSpace(gr.Choices[0].Message.Content)
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)

		var errCheck struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(content), &errCheck) == nil && errCheck.Error != "" {
			c.String(http.StatusOK, modalErro("Não consegui identificar uma movimentação financeira no texto. Tente descrever um gasto ou receita."))
			return
		}

		var transactions []map[string]interface{}
		if err := json.Unmarshal([]byte(content), &transactions); err != nil {
			c.String(http.StatusOK, modalErro("A IA retornou um formato inesperado. Tente novamente."))
			return
		}

		for _, transaction := range transactions {
			jsonData, _ := json.Marshal(transaction)
			apiResp, err := http.Post(
				"https://apifinanceiro-aeeo.onrender.com/api/v1/transactions",
				"application/json",
				bytes.NewBuffer(jsonData),
			)
			if err != nil || (apiResp.StatusCode != http.StatusOK && apiResp.StatusCode != http.StatusCreated) {
				c.String(http.StatusOK, modalErro("Erro ao salvar uma das movimentações na API."))
				return
			}
			apiResp.Body.Close()
		}

		c.Header("HX-Redirect", "/dashboard")
		c.String(http.StatusOK, "")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Servidor rodando na porta", port)
	r.Run(":" + port)
}
