# Ревью лендинга Pulse

Сопоставлено с локальным `main` на коммите `ff30cd6` и веткой `gh-pages` на `b4bcc32`.

## Найдено и исправлено

| Приоритет | Наблюдение | Изменение |
| --- | --- | --- |
| Высокий | Не представлены heartbeat, история, MCP, локализация и роли пользователей | Обновлены карточки; добавлен раздел heartbeat, Quick Start и FAQ. Heartbeat и роли уже входят в обновлённый main. |
| Высокий | Обещаны configurable reminders, хотя подключение напоминаний не завершено | Убрано обещание; описаны фактически работающие уведомления. |
| Высокий | Zero external dependencies противоречит необходимости PostgreSQL и TimescaleDB | Требования к базе явно указаны в описании, карточке, инструкции и FAQ. |
| Средний | Нет robots.txt и sitemap.xml | Добавлены файлы с единым canonical `https://uptime-pulse.com/`. Sitemap включает главную и `/get-started/` и помогает обнаружению, но не является обязательным условием индексации. |
| Средний | Слишком мало полезного текста о сценариях использования | Добавлены конкретные сценарии website/API/infrastructure/cron monitoring, вопросы и инструкция запуска; весь текст доступен без JavaScript. |
| Средний | Нет структурированного описания продукта | Добавлен JSON-LD WebSite + SoftwareApplication. Нет выдуманных рейтингов. Это описание сущностей, а не обещание расширенного результата Google. |
| Средний | Внешние Google Fonts создают дополнительные подключения и блокирующий CSS | Используются системные шрифты, внешняя таблица стилей удалена. Изменение метрик скорости пока не измерялось. |
| Средний | Белые подписи на ярком голубом фоне, недостаточно читаемый вторичный текст | Разделены цвета текста и заливки кнопок; улучшен контраст. Полный WCAG-аудит не проводился. |
| Средний | Нет main/skip link/focus-visible/reduced-motion; тесные строки на мобильном | Добавлены ориентиры, фокус, отключение анимаций и таймера по reduced-motion, переносы в узких строках. Требуется визуальная проверка. |
| Низкий | Иллюстрация похожа на реальные live-метрики | Данные явно подписаны как демонстрационные. |
| Низкий | Нет обычного файла favicon | Добавлены существующие брендовые PNG favicon и Apple touch icon. |
| Низкий | Meta keywords не помогает Google | Тег удалён; уточнены title, description и социальные метаданные. |

Google Search Console verification сохранён. Добавлены CNAME и .nojekyll для статической публикации на существующем домене.

## Что сделать после публикации

1. В Google Search Console отправить `https://uptime-pulse.com/sitemap.xml`, выполнить URL Inspection главной и запросить переиндексацию. Тег подтверждения уже есть, но доступ и состояние Search Console не проверялись.
2. Проверить HTTPS и редиректы HTTP → HTTPS, www → основной домен, GitHub Pages URL → основной домен. Для CNAME на GitHub Pages проверить настройки custom domain и Enforce HTTPS. Файл CNAME сам по себе не подтверждает настройки хостинга или DNS.
3. Проверить реальные HTTP-ответы главной, robots.txt, sitemap.xml, og.png и favicon после деплоя. Публичная страница была доступна через веб-инструмент; ответы robots/sitemap до правок инструмент получить не смог. Их прежние HTTP-статусы не установлены.
4. Запустить Lighthouse/PageSpeed Insights для мобильного и десктопа; проверить Core Web Vitals в Search Console, когда появятся полевые данные. Локальные DOM-проверки не измеряют LCP, INP, CLS или визуальную вёрстку.
5. Проверить страницу в Schema Markup Validator. SoftwareApplication без настоящего review/aggregateRating не удовлетворяет всем требованиям Google для software-app rich result. Не добавлять искусственные отзывы ради валидатора. FAQ оставлен полезным HTML-контентом: FAQ rich results Google ограничивает авторитетными государственными и медицинскими сайтами.

## Следующие задачи для роста органического трафика

- Создать самостоятельные содержательные страницы `/docs/docker/`, `/monitoring/http/`, `/monitoring/grpc/`, `/monitoring/cron-jobs/`: рабочие примеры, ограничения, диагностика ошибок и ссылки на исходники. У каждой страницы должны быть свой title, description, canonical и запись в sitemap. Разделы одной страницы не заменяют отдельные URL для разных поисковых намерений.
- Добавить реальные актуальные скриншоты dashboard и heartbeat timeline с размерами, сжатием и описательными alt. Текущий блок — иллюстрация, не скриншот продукта.
- После появления переведённого контента завести отдельные языковые URL и взаимные hreflang. 13 языков интерфейса приложения не означают, что лендинг уже переведён.
- Публиковать технические руководства и заметки о релизах; связывать их с README и документацией. Сравнения с другими продуктами должны опираться на проверенные актуальные возможности.
- Через 4–6 недель после индексации оценить запросы, показы, CTR и позиции в Search Console. Выбирать следующие страницы по реальным запросам и сценарию пользователя.

## Выполненная проверка

- DOM: один h1 и main, уникальные id, локальные якоря и файлы ресурсов.
- Метаданные: единственный canonical, соответствие og:url, сохранённое подтверждение Google.
- JSON-LD: корректный JSON; sitemap: корректный XML и canonical URL; robots: ссылка на sitemap.
- Inline JavaScript: синтаксис, полный цикл тем, недоступный localStorage, reduced-motion без таймера.
- Размеры PNG: OG 1200×630, favicon 32×32, Apple touch icon 180×180.
- `git diff --check`.

Визуальная проверка браузером, Lighthouse, проверка внешних ссылок по HTTP и интерфейс Search Console не выполнялись. Фактические позиции в поиске не измерялись. Правки подготовлены локально, без публикации.

## Источники

- [Google: руководство для разработчиков](https://developers.google.com/search/docs/fundamentals/get-started-developers)
- [Canonical URL](https://developers.google.com/search/docs/crawling-indexing/consolidate-duplicate-urls)
- [Создание sitemap](https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap)
- [Описание поискового сниппета](https://developers.google.com/search/docs/appearance/snippet)
- [Поддерживаемые meta-теги; keywords не влияет на ранжирование](https://developers.google.com/search/docs/crawling-indexing/special-tags)
- [SoftwareApplication и требования Google](https://developers.google.com/search/docs/appearance/structured-data/software-app)
- [Ограничения FAQ rich results](https://developers.google.com/search/blog/2023/08/howto-faq-changes)

## Отдельная страница запуска

Инструкция перенесена с главной на `/get-started/`. На главной остаются CTA и навигационная ссылка. У инструкции собственные h1, title, description, canonical, социальные метаданные, WebPage JSON-LD и запись sitemap. Добавлены шаги настройки секретов, запуска и создания первого монитора.
