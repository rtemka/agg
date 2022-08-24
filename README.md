## Пример приложения с микросервисной архитектурой.

#### **Состав приложения:**
1. [Сервис агрегации новостей.](https://gitlab.com/rtemka/newsservice)
2. [Сервис комментариев к новостям.](https://gitlab.com/rtemka/comments)
3. [Сервис модерации комментариев.](https://gitlab.com/rtemka/commscheck)
4. [API Gateway.](https://gitlab.com/rtemka/gateway)

#### **Запуск**

Предполагается, что на машине установлен **Docker**.

```bash
git clone https://github.com/rtemka/agg.git
cd ./agg
docker compose up
```

