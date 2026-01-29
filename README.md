# Distributed Task Queue partitioning and Rebalancing

## 🇺🇸 EN


## 🇧🇷 PT-BR

## O que é

É uma fila de tarefas distribuídas, que automaticamente se particiona e faz rebalanceamento.

Aqui, vamos ter múltiplas filas no Redis, e vamos ter workers em processos diferentes, se coordenando simultaneamente. Um worker vai pegar uma tarefa da fila, vai processar e depois pegar a próxima.

### <span style="color:rgb(0, 255, 136)">Cenário</span>

Imagina que temos um sistema que precisa processar **milhares de tarefas por segundo.** Como:

- Processar imagens
- Enviar e-mails
- Gerar relatórios

Qualquer tarefa que pode demorar algum tempo. Se fizermos isso em apenas um servidores, ele não daria conta, ia entrar em sobrecarga, dependendo do nível de escala. Então, precisamos distribuir esse trabalho entre vários servidores.

### <span style="color:rgb(0, 255, 136)">Problemas</span>

Mas existem **problemas**:

- Como coordenar isso?
- Como garantimos que cada tarefa seja processada exatamente uma vez?
- Como dividimos o trabalho de forma justa entre os servidores?
- O que acontece quando um servidor morre ou quando adicionamos servidores novos?

### Abordagens possíveis

Podemos resolver esses problemas das seguintes formas:

1. Ter um Coordenador central, uma entidade centralizada que coordena todos os trabalhos. Porém tem um problema nessa abordagem, essa entidade central é um ponto único de falha e um gargalo ao sistema. Se ele falhar o sistema todo falha e perdemos em **disponibilidade.** (availability - CAP)
2. A segunda abordagem é ter um sistema onde os próprios workers se coordenam entre si, sem entidade central, se rebalanceando de forma dinâmica e inteligente. Essa é a abordagem desse projeto.

### Ideias Centrais desse projeto

Aqui temos 3 ideias centrais:

1. Usar **particionamento:** dividir todas as tarefas possíveis em 256 grupos fixos, chamados **partitions**.
2. Usar **consistent hashing:** para decidir qual worker é responsável por quais partitions de forma determinística. Ou seja, um roteamento preciso para o sistema.
3. Usar **etcd**: para os workers se descobrirem e coordenarem quem está vivo no cluster.

Dessa forma, os **workers** podem olhar ao etcd e ver quem está vivo, roda o mesmo algoritmo de consistent hashing e todos chegam na mesma conclusão sobre quem deve processar quais partitions. É um **consenso - consensus protocol**.