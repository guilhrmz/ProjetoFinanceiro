# Nexo — Controle Financeiro Pessoal

Aplicação web de gestão financeira pessoal construída com Go e HTMX. Permite registrar entradas e saídas, visualizar resumos por período, analisar gastos por categoria em um gráfico interativo e exportar os dados em CSV. Conta ainda com um assistente de IA que interpreta texto livre e cria movimentações automaticamente.

---

## Funcionalidades

- **Autenticação** — cadastro e login com sessão segura via cookie criptografado
- **Dashboard** — cards de totais (entradas, saídas e saldo), tabela de movimentações e gráfico de pizza por categoria de saída
- **Filtro por período** — 7, 15 ou 30 dias, ou todas as movimentações; filtra tabela e gráfico ao mesmo tempo
- **Registro manual** — formulário na sidebar com valor, categoria e tipo (entrada/saída); data registrada automaticamente no momento do envio
- **Registro com IA** — descreva a movimentação em linguagem natural (ex: *"gastei 80 reais no mercado"*) e a IA classifica categoria, tipo e valor automaticamente via LLaMA 3.1
- **Exportação CSV** — baixa todas as movimentações do usuário em `ID,Categoria,Tipo,Data,Valor`

---

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | [Go 1.26](https://go.dev) + [Gin](https://github.com/gin-gonic/gin) |
| Frontend | HTML + CSS + [HTMX 1.9](https://htmx.org) |
| Sessões | [Gorilla Sessions](https://github.com/gorilla/sessions) (cookie criptografado) |
| IA | [Groq API](https://groq.com) — modelo `llama-3.1-8b-instant` |
| API de dados | REST externa (Go + MongoDB) hospedada no Render |
| Hospedagem | [Render](https://render.com) |

---

## Estrutura do projeto

```
ProjetoFinanceiro/
├── login/
│   ├── main.go        # servidor principal — todas as rotas
│   └── style.css      # estilos das páginas de autenticação
├── main/
│   ├── index.html     # template do dashboard (Go template)
│   └── style.css      # estilos do dashboard
├── go.mod
├── go.sum
└── .vscode/
    └── launch.json    # configuração de debug local
```

> O servidor em `login/main.go` é o único ponto de entrada. A pasta `main/` contém apenas o template HTML e os estilos do dashboard.

---

## Rotas

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/` | Página de login |
| `GET` | `/cadastro` | Página de cadastro |
| `POST` | `/cadastro` | Cria conta na API |
| `POST` | `/login` | Autentica e inicia sessão |
| `GET` | `/sair` | Encerra sessão e redireciona para `/` |
| `GET` | `/dashboard` | Dashboard principal (requer sessão) |
| `GET` | `/dashboard?dias=N` | Dashboard filtrado por período (7, 15 ou 30 dias) |
| `POST` | `/transaction` | Registra nova movimentação |
| `GET` | `/exportar-csv` | Baixa movimentações do usuário em CSV |
| `GET` | `/modal-ia` | Abre modal do assistente de IA |
| `POST` | `/ia` | Processa texto com IA e salva movimentações |

---

## Variáveis de ambiente

| Variável | Descrição | Obrigatória |
|---|---|---|
| `API_BASE_URL` | URL base da API de dados (sem barra final) | Sim |
| `SESSION_SECRET` | Chave de criptografia das sessões | Sim |
| `GROQ_API_KEY` | Chave da API Groq para o assistente de IA | Sim |
| `PORT` | Porta do servidor (padrão: `8080`) | Não |

---

## Rodando localmente

### Pré-requisitos

- [Go 1.21+](https://go.dev/dl/)
- Conta na [Groq](https://console.groq.com) para obter a chave de API

### Instalação

```bash
# Clone o repositório
git clone https://github.com/guilhrmz/ProjetoFInanceiro.git
cd ProjetoFInanceiro

# Instale as dependências
go mod download
```

### Configuração

Defina as variáveis de ambiente ou adicione-as em `.vscode/launch.json` (já configurado para debug):

```json
"env": {
  "GIN_MODE": "debug",
  "API_BASE_URL": "https://sua-api.onrender.com",
  "SESSION_SECRET": "chave-secreta-local",
  "GROQ_API_KEY": "sua-chave-groq"
}
```

### Executando

```bash
# A partir da raiz do projeto
go run ./login
```

Acesse `http://localhost:8080`.

---

## Deploy no Render

1. Conecte o repositório ao Render como **Web Service**
2. Configure o **Build Command**: `go build -o app ./login`
3. Configure o **Start Command**: `./app`
4. Adicione as variáveis de ambiente no painel do Render:
   - `API_BASE_URL`
   - `SESSION_SECRET`
   - `GROQ_API_KEY`

> O servidor usa a variável `PORT` injetada automaticamente pelo Render.

---

## Como a IA funciona

O assistente recebe texto livre do usuário e envia para o modelo `llama-3.1-8b-instant` via Groq com um prompt de sistema que instrui o modelo a identificar valor, tipo (entrada/saída) e categoria. A resposta é um array JSON que é validado e salvo diretamente na API de dados. Textos sem valor numérico ou ação financeira clara são rejeitados com mensagem de erro.

**Categorias reconhecidas:** Alimentação, Transporte, Moradia, Salário, Lazer, Outros.
