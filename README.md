# Distributed Task Queue partitioning and Rebalancing

🇺🇸 A self coordinating distributed task processing system using consistent hashing for dynamic partition assignment and etcd for membership coordination of agents / nodes.

[🇺🇸 English](#-english) | [🇧🇷 Português](#-português)

## 🇺🇸 English

### Overview

A distributed task queue where multiple worker processes are self coordinate to process tasks from Redis queues without a central scheduler.

Workers dynamically discover each other using etcd and use consistent hashing to deterministically decide partition ownership, enabling automatic rebalancing when workers join or leave the cluster.

### Demo

**TODO**

### Archicture

**TODO**


### Quick Start

**Prerequisites**
- Docker and Docker Compose
- Go 1.2x.+

**Setup**
```bash
# start redis and etcd
make setup

# build and run worker:
make execute

# this command create tasks:
make create-tasks
```

**Run Multiple Workers**
```bash
# terminal 1
make execute

# terminal 2  
make execute

# terminal 3
make execute

```

**Clean Up**
```bash
make clean
```

#### Core Components

**Redis (Task Storage - queue)**

- 256 fixed partitions (`tasks:0` ... `tasks:255`)
- Tasks hashed by ID to determine partition placement
- Workers use BLPOP for atomic task retrical

**etcd (Membership Coordination)**

- Workers register with 10 second TTL leases
- Continuous lease renewal with KeepAlive
- Watch mechanism detects if a member joins or leaves etcd in real time
- No central coordinator required (furthermore no single point of failure)

**Consistent Hash Ring (partition assignment to workers)**

- 120 virtual nodes for each worker for statistical distribution
- Deterministic partition ownership calculation
- Minimal partition movement during rebalancing (~1/N partitions move when cluster size changes)
- All workers independently will reach the same conclusion about ownership

**Workers (task processors)**

- Self register on startup with a unique ID
- Calculate owned partitions via consistent hashing
- Process tasks from owned partitions using redis BLPOP
- Automatically rebalance when membership changes
- Graceful shutdown with lease revocation

### Key Features

- **Decentralized Coordination** - no single point of failure (a tradeoff from having a central manager of workers...)
- **Dynamic Rebalacing** - Automatic partition redistribution on membership changes
- **At-least-once delivery** - Task guaranteed to be processed even during failures
- **Partition Isolation** - Each task mapped to exactly one partition
- **Graceful Shutdown** - Workers revoke leases before stopping

### Why these Choices

**Why Consistent Hashing**

- **Minimal Movement**: consistent hashing allows only ~1 / N reassigned when adding or removing workers
- **Deterministic**: All workers calculate the same partitions ownership independently
- **No coordination overhead**: No need to communicate partition assignments
- Other approaches (random assignment, range-based, static hashing) either lack dynamic redistribution or require centralized coordination

**Why 256 Fixed Partitions?**

- At I used module on the hash of the task_id to assing the partitions, and as a number of power of 2 enables efficient modulo operations. Altought later on I switched to always get the first byte of the hash, which is still 256 possible combinations. I had to switch because the modulo operator was overflowing the integer.
- It provides a fine grained distribution even with few workers
- Fixed count simplifies reasoning about system behavior
- Its large enough to minime any hot spots in partition distribution across workers, and small enough to avoid unnecessary overhead

**Why pull model (BLPOP)**

- Self-Balancing: fast workers can process more tasks
- No capacity tracking: workers can pull at their own pace
- Atomic operations: Redis BLPOP guaranteers atomic operations, which means one worker gets each task
- Blocks efficiently: no busy waiting or polling overhead

**Why 120 Virtual Nodes?**

- Statistical uniformity: it reduces distribution variance to ~10-15%
- Tested sweet spot: fewer vnodees will lead to uneven distribution, more vnodes = unnecessary overhead

**Why etcd Over Redis for Membership?**

- Etcd is designed for distributed coordination with strong consistency (used by K8s internally)
- It have a built-in lease mechanism with automatic expiration
- The watch API is ideal for tracking real time membership updates
- Raft consensus implemented internally for reliable failure detection

### Technical Challenges Solved

**Race Condition in Rebalancing**

- **Problem**: Multiple goroutines created for each `RunTask()` call competed for rebalance signals, causing goroutine leaks and delayed cancellations
- **Solution**: Single persistent goroutine listens on `updateChan`, cancels processing context and will recreate context after cancellation

**Context Lifecycle Management**

- **Problem**: Reusing cancelled context caused subsequent BLPOP calls to fail immediately
- **Solution**: Recreate context after each cancellation with mutex protection for thread safety

**Graceful Shutdown**

- **Problem**: Workers crashed without notifying cluster, leaving partitions unprocessed until lease expiry (10s)
- **Solution**: Explicit lease revocation on shutdown for immediate partition reassignment

### How It Works

**1. Task Submission**
```
Task with ID -> Hash(ID) -> partition = hash[0] -> Push to redis list tasks:n
```

**2. Worker Startup**
```
Worker starts -> Register in etcd with lease -> Add self to hash ring ->
Calculate owned partitions -> Start BLPOP on owned partition queues
```

**3. Task Processing**
```
BLPOP blocks on owned partitions -> Task arrives -> Process ->
Loop back to BLPOP (continuous processing)
```

**4. Rebalancing**
```
etcd watch detects change -> Update hash ring -> Recalculate partitions ->
Cancel current BLPOP -> Start BLPOP on new partitions
```

**5. Graceful Shutdown**
```
SIGTERM received -> Cancel processing context -> Revoke etcd lease ->
Close connections -> Exit
```

### Project Structure

```
├── cmd/
│   ├── worker/          # worker main entry point
│   └── cliTasks/        # cli tool to send tasks
├── internal/
│   ├── worker/          # worker logic & coordination
│   ├── ring/            # consistent hash ring implementation
│   ├── conn/            # redis & etcd connection management
│   └── types/           # shared types
├── docker-compose.yml   # redis & etcd services
└── Makefile            # build & run commands
```

### Future Improvements

- [ ] Exponential backoff retry logic
- [ ] Dead letter queue for failed tasks
- [ ] Prometheus metrics (tasks processed, partition ownership, rebalance events)
- [ ] Health check endpoint
- [ ] Tests for membership changes
- [ ] Configurable partition count and virtual nodes
- [ ] A priotity queue for tasks (would be nice to have such)

### Technical Stack

- **Language**: Go 1.21
- **Coordination**: etcd v3.6.7
- **Storage**: Redis 7.x
- **Hashing**: MurmurHash3 (high performance and low collision)
- **Concurrency**: goroutines, channels, mutex, context, sync primitives

## 🇧🇷 Português

### Visão geral

Um sistema de processamento de tarefas auto coordenado, usando consistent hashing para atribuição de partições de forma dinâmica e etcd para coordernação de adesão de agents / nodes (workers).

Aqui, vamos ter múltiplas filas no Redis, e vamos ter workers em processos diferentes, se coordenando simultaneamente. Um worker vai pegar uma tarefa da fila, vai processar e depois pegar a próxima.

### Demonstração

**TODO**: Adicionar vídeo de demonstração

### Arquitetura

**TODO**: Adicionar diagrama de arquitetura

#### Componentes Principais

**Redis (armazenamento de tarefas - fila)**

Redis guarda 256 filas separadas chamadas `tasks:0` até `tasks:255`. Quando uma tarefa chega, fazemos hash do ID dela e usamos o primeiro byte desse hash para descobrir em qual fila ela vai (sempre terá um valor entre 0 e 255). Os workers usam BLPOP, que é uma operação atômica do Redis que pega uma tarefa da fila e bloqueia esperando se a fila estiver vazia.

**etcd (coordenação de membros)**

Cada worker se registra no etcd com um lease de 10 segundos que fica sendo renovado continuamente através do KeepAlive. O etcd tem um mecanismo de watch que notifica em tempo real quando algum worker entra ou sai. Não existe coordenador central, então não tem ponto único de falha.

**Consistent Hash Ring (atribuição de partições aos workers)**

O hash ring usa 120 nós virtuais para cada worker para melhorar a distribuição estatística das partições. Todos os workers fazem o mesmo cálculo de forma independente e chegam na mesma conclusão sobre quem é dono de quais partições. Quando o tamanho do cluster muda, apenas cerca de 1/N das partições precisam ser movidas para outros workers.

**Workers (processadores de tarefas)**

Cada worker se registra na inicialização com um ID único, calcula quais partições ele deve processar usando consistent hashing, e começa a fazer BLPOP nessas partições. Quando a composição do cluster muda, ele automaticamente recalcula suas partições. No shutdown, ele revoga seu lease do etcd antes de parar.

### Funcionalidades Principais

**Coordenação descentralizada**: não existe ponto único de falha porque não tem gerenciador central de workers
**Rebalanceamento dinâmico**: quando workers entram ou saem, as partições são redistribuídas automaticamente
**Entrega ao menos uma vez**: tarefas são garantidas de serem processadas mesmo quando acontecem falhas
**Isolamento de partições**: cada tarefa vai para exatamente uma partição específica
**Shutdown gracioso**: workers revogam seus leases antes de parar

### Por Que Essas Escolhas

**Por que Consistent Hashing**

- Quantidade mínima de operações: consistent hashing garante que apenas cerca de 1/N das partições sejam reatribuídas quando adicionamos ou removemos workers
- Determinístico: todos os workers calculam as mesmas atribuições de partições de forma independente
- Sem overhead de coordenação: não precisa comunicar atribuições de partições entre workers
- Outras abordagens como random distribution, baseada em ranges, ou static hashing ou não permitem redistribuição dinâmica ou precisam de coordenação centralizada

**Por que 256 partições fixas**

- Inicialmente eu estava usando módulo no hash do task_id para atribuir partições, e como 256 é potência de 2, isso permite operações de módulo eficientes. Depois mudei para sempre pegar o primeiro byte do hash, que ainda dá 256 combinações possíveis. Tive que mudar porque a operação de módulo estava causando overflow no inteiro
- Fornece distribuição granular mesmo com poucos workers
- Número fixo simplifica o raciocínio sobre comportamento do sistema
- É grande o suficiente para minimizar pontos quentes na distribuição de partições entre workers, e pequeno o suficiente para evitar overhead desnecessário

**Por que modelo pull com BLPOP**

- Auto balanceamento: workers rápidos naturalmente processam mais tarefas
- Sem rastreamento de capacidade: workers pegam tarefas no próprio ritmo
- Operações atômicas: Redis BLPOP garante operações atômicas, ou seja, apenas um worker pega cada tarefa
- Bloqueia eficientemente: não tem busy waiting nem overhead de polling

**Por que 120 nós virtuais**

- Uniformidade estatística: reduz variação de distribuição para cerca de 10 a 15%
- Ponto ideal: menos nós virtuais leva a distribuição desigual, mais nós virtuais causa muito overhead

**Por que etcd ao invés de Redis para membership**

- Etcd foi projetado para coordenação distribuída com consistência forte (usado internamente pelo K8s)
- Tem mecanismo de lease integrado com expiração automática
- A API de watch é ideal para rastrear atualizações de membership em tempo real
- Raft Consensus implementado internamente para detectar de falhas

### Desafios Técnicos Resolvidos

**Race condition no rebalanceamento**

**Problema**: múltiplas goroutines eram criadas a cada chamada de `RunTask()` e competiam pelos sinais de rebalanceamento, causando vazamento de goroutines e cancelamentos atrasados
**Solução**: uma única goroutine persistente escuta no `updateChan` cancela o contexto de processamento e recria o contexto após cancelamento

**Gerenciamento de ciclo de vida do contexto**

**Problema**: reusar contexto cancelado fazia as próximas chamadas de BLPOP falharem imediatamente
**Solução**: recriar contexto após cada cancelamento com proteção de mutex para thread safety

**Shutdown gracioso**

**Problema**: workers crashavam sem notificar o cluster, deixando partições sem processar até o lease expirar (10 segundos)
**Solução**: revogação explícita do lease no shutdown para reatribuição imediata de partições

### Como Começar

**Pré requisitos**

Docker e Docker Compose

Go 1.2x ou superior

## License

MIT License - feel free to use this project for learning and interviews.

## Author

Victor Reis - [my website](https://viquitorreis.github.io/)