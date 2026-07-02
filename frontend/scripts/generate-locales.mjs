import { readFileSync, writeFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const localesDir = resolve(__dirname, '../src/locales');
const en = JSON.parse(readFileSync(resolve(localesDir, 'en.json'), 'utf-8'));

const translations = {
  ru: {
    nav: {
      dashboard: "Панель мониторинга",
      monitors: "Мониторы",
      settings: "Настройки",
      logout: "Выйти",
      home: "Pulse — Главная"
    },
    common: {
      save: "Сохранить",
      cancel: "Отмена",
      delete: "Удалить",
      confirm: "Подтвердить",
      retry: "Повторить",
      apply: "Применить",
      back: "Назад",
      loading: "Загрузка...",
      error: "Что-то пошло не так",
      copy: "Копировать",
      copied: "Скопировано!",
      remove: "Убрать",
      replace: "Заменить",
      add: "Добавить",
      none: "Нет",
      never: "Никогда",
      na: "Н/Д",
      active: "Активен",
      paused: "Приостановлен",
      saving: "Сохранение…",
      creating: "Создание…",
      deleting: "Удаление…",
      revoking: "Отзыв…",
      pageOf: "Страница {page} из {totalPages}",
      previous: "Предыдущая",
      next: "Следующая"
    },
    connection: {
      live: "Подключено",
      reconnecting: "Переподключение…",
      paused: "Обновления приостановлены",
      statusLive: "Статус соединения: подключено",
      statusPaused: "Статус соединения: обновления приостановлены"
    },
    settings: {
      title: "Настройки",
      description: "Управление учётной записью и доступом к API.",
      language: { title: "Язык", label: "Язык интерфейса" },
      tokens: {
        title: "API-токены",
        description: "API-токены обеспечивают программный доступ к API Pulse. Используйте их для CI/CD, скриптов автоматизации и внешних интеграций.",
        createTitle: "Создать новый токен",
        createLabel: "Имя токена",
        createPlaceholder: "например ci-deploy, grafana-read",
        createButton: "Создать токен",
        existingTitle: "Существующие токены",
        existingDescription: "Значения токенов не отображаются после создания. Показаны только метаданные.",
        loadingTokens: "Загрузка токенов…",
        emptyState: "API-токены ещё не созданы.",
        tableHeaders: { name: "Имя", created: "Создан", lastUsed: "Последнее использование", expires: "Истекает", actions: "Действия" },
        revoke: "Отозвать",
        revoked: "Отозван",
        expired: "Истёк"
      }
    },
    "login.title": "Вход в Pulse",
    "login.subtitle": "Введите учётные данные для продолжения",
    "login.submit": "Войти",
    "dashboard.title": "Панель мониторинга"
  },
  es: {
    nav: {
      dashboard: "Panel de control",
      monitors: "Monitores",
      settings: "Configuración",
      logout: "Cerrar sesión",
      home: "Pulse — Inicio"
    },
    common: {
      save: "Guardar",
      cancel: "Cancelar",
      delete: "Eliminar",
      confirm: "Confirmar",
      retry: "Reintentar",
      apply: "Aplicar",
      back: "Atrás",
      loading: "Cargando...",
      error: "Algo salió mal",
      copy: "Copiar",
      copied: "¡Copiado!",
      remove: "Quitar",
      replace: "Reemplazar",
      add: "Agregar",
      none: "Ninguno",
      never: "Nunca",
      na: "N/D",
      active: "Activo",
      paused: "Pausado",
      saving: "Guardando…",
      creating: "Creando…",
      deleting: "Eliminando…",
      revoking: "Revocando…",
      pageOf: "Página {page} de {totalPages}",
      previous: "Anterior",
      next: "Siguiente"
    },
    connection: {
      live: "En vivo",
      reconnecting: "Reconectando…",
      paused: "Actualizaciones en pausa",
      statusLive: "Estado de conexión: en vivo",
      statusPaused: "Estado de conexión: actualizaciones en pausa"
    },
    settings: {
      title: "Configuración",
      description: "Administra tu cuenta y acceso a la API.",
      language: { title: "Idioma", label: "Idioma de visualización" },
      tokens: {
        title: "Tokens de API",
        description: "Los tokens de API proporcionan acceso programático a la API de Pulse. Úsalos para pipelines CI/CD, scripts de automatización e integraciones externas.",
        createTitle: "Crear nuevo token",
        createLabel: "Nombre del token",
        createPlaceholder: "ej. ci-deploy, grafana-read",
        createButton: "Crear token",
        existingTitle: "Tokens existentes",
        existingDescription: "Los valores de los tokens no se muestran después de la creación. Solo se muestran los metadatos.",
        loadingTokens: "Cargando tokens…",
        emptyState: "Aún no se han creado tokens de API.",
        tableHeaders: { name: "Nombre", created: "Creado", lastUsed: "Último uso", expires: "Expira", actions: "Acciones" },
        revoke: "Revocar",
        revoked: "Revocado",
        expired: "Expirado"
      }
    },
    "login.title": "Iniciar sesión en Pulse",
    "login.subtitle": "Ingrese sus credenciales para continuar",
    "login.submit": "Iniciar sesión",
    "dashboard.title": "Panel de control"
  },
  fr: {
    nav: {
      dashboard: "Tableau de bord",
      monitors: "Moniteurs",
      settings: "Paramètres",
      logout: "Déconnexion",
      home: "Pulse — Accueil"
    },
    common: {
      save: "Enregistrer",
      cancel: "Annuler",
      delete: "Supprimer",
      confirm: "Confirmer",
      retry: "Réessayer",
      apply: "Appliquer",
      back: "Retour",
      loading: "Chargement...",
      error: "Une erreur est survenue",
      copy: "Copier",
      copied: "Copié !",
      remove: "Retirer",
      replace: "Remplacer",
      add: "Ajouter",
      none: "Aucun",
      never: "Jamais",
      na: "N/D",
      active: "Actif",
      paused: "En pause",
      saving: "Enregistrement…",
      creating: "Création…",
      deleting: "Suppression…",
      revoking: "Révocation…",
      pageOf: "Page {page} sur {totalPages}",
      previous: "Précédent",
      next: "Suivant"
    },
    connection: {
      live: "En direct",
      reconnecting: "Reconnexion…",
      paused: "Mises à jour en pause",
      statusLive: "État de la connexion : en direct",
      statusPaused: "État de la connexion : mises à jour en pause"
    },
    settings: {
      title: "Paramètres",
      description: "Gérez votre compte et l'accès à l'API.",
      language: { title: "Langue", label: "Langue d'affichage" },
      tokens: {
        title: "Jetons API",
        description: "Les jetons API fournissent un accès programmatique à l'API Pulse. Utilisez-les pour les pipelines CI/CD, les scripts d'automatisation et les intégrations externes.",
        createTitle: "Créer un nouveau jeton",
        createLabel: "Nom du jeton",
        createPlaceholder: "ex. ci-deploy, grafana-read",
        createButton: "Créer le jeton",
        existingTitle: "Jetons existants",
        existingDescription: "Les valeurs des jetons ne sont plus affichées après la création. Seules les métadonnées sont visibles.",
        loadingTokens: "Chargement des jetons…",
        emptyState: "Aucun jeton API créé pour le moment.",
        tableHeaders: { name: "Nom", created: "Créé", lastUsed: "Dernière utilisation", expires: "Expire", actions: "Actions" },
        revoke: "Révoquer",
        revoked: "Révoqué",
        expired: "Expiré"
      }
    },
    "login.title": "Connexion à Pulse",
    "login.subtitle": "Entrez vos identifiants pour continuer",
    "login.submit": "Se connecter",
    "dashboard.title": "Tableau de bord"
  },
  pt: {
    nav: {
      dashboard: "Painel",
      monitors: "Monitores",
      settings: "Configurações",
      logout: "Sair",
      home: "Pulse — Início"
    },
    common: {
      save: "Salvar",
      cancel: "Cancelar",
      delete: "Excluir",
      confirm: "Confirmar",
      retry: "Tentar novamente",
      apply: "Aplicar",
      back: "Voltar",
      loading: "Carregando...",
      error: "Algo deu errado",
      copy: "Copiar",
      copied: "Copiado!",
      remove: "Remover",
      replace: "Substituir",
      add: "Adicionar",
      none: "Nenhum",
      never: "Nunca",
      na: "N/D",
      active: "Ativo",
      paused: "Pausado",
      saving: "Salvando…",
      creating: "Criando…",
      deleting: "Excluindo…",
      revoking: "Revogando…",
      pageOf: "Página {page} de {totalPages}",
      previous: "Anterior",
      next: "Próximo"
    },
    connection: {
      live: "Ao vivo",
      reconnecting: "Reconectando…",
      paused: "Atualizações pausadas",
      statusLive: "Status da conexão: ao vivo",
      statusPaused: "Status da conexão: atualizações pausadas"
    },
    settings: {
      title: "Configurações",
      description: "Gerencie sua conta e acesso à API.",
      language: { title: "Idioma", label: "Idioma de exibição" },
      tokens: {
        title: "Tokens de API",
        description: "Tokens de API fornecem acesso programático à API do Pulse. Use-os para pipelines CI/CD, scripts de automação e integrações externas.",
        createTitle: "Criar novo token",
        createLabel: "Nome do token",
        createPlaceholder: "ex. ci-deploy, grafana-read",
        createButton: "Criar token",
        existingTitle: "Tokens existentes",
        existingDescription: "Os valores dos tokens não são exibidos após a criação. Apenas os metadados são mostrados.",
        loadingTokens: "Carregando tokens…",
        emptyState: "Nenhum token de API criado ainda.",
        tableHeaders: { name: "Nome", created: "Criado", lastUsed: "Último uso", expires: "Expira", actions: "Ações" },
        revoke: "Revogar",
        revoked: "Revogado",
        expired: "Expirado"
      }
    },
    "login.title": "Entrar no Pulse",
    "login.subtitle": "Insira suas credenciais para continuar",
    "login.submit": "Entrar",
    "dashboard.title": "Painel"
  },
  de: {
    nav: {
      dashboard: "Übersicht",
      monitors: "Monitore",
      settings: "Einstellungen",
      logout: "Abmelden",
      home: "Pulse — Startseite"
    },
    common: {
      save: "Speichern",
      cancel: "Abbrechen",
      delete: "Löschen",
      confirm: "Bestätigen",
      retry: "Erneut versuchen",
      apply: "Anwenden",
      back: "Zurück",
      loading: "Laden...",
      error: "Etwas ist schiefgelaufen",
      copy: "Kopieren",
      copied: "Kopiert!",
      remove: "Entfernen",
      replace: "Ersetzen",
      add: "Hinzufügen",
      none: "Keine",
      never: "Nie",
      na: "k. A.",
      active: "Aktiv",
      paused: "Pausiert",
      saving: "Speichern…",
      creating: "Erstellen…",
      deleting: "Löschen…",
      revoking: "Widerrufen…",
      pageOf: "Seite {page} von {totalPages}",
      previous: "Zurück",
      next: "Weiter"
    },
    connection: {
      live: "Verbunden",
      reconnecting: "Verbindung wird wiederhergestellt…",
      paused: "Live-Updates pausiert",
      statusLive: "Verbindungsstatus: verbunden",
      statusPaused: "Verbindungsstatus: Live-Updates pausiert"
    },
    settings: {
      title: "Einstellungen",
      description: "Verwalten Sie Ihr Konto und den API-Zugang.",
      language: { title: "Sprache", label: "Anzeigesprache" },
      tokens: {
        title: "API-Tokens",
        description: "API-Tokens ermöglichen programmatischen Zugriff auf die Pulse-API. Verwenden Sie sie für CI/CD-Pipelines, Automatisierungsskripte und externe Integrationen.",
        createTitle: "Neuen Token erstellen",
        createLabel: "Token-Name",
        createPlaceholder: "z.B. ci-deploy, grafana-read",
        createButton: "Token erstellen",
        existingTitle: "Vorhandene Tokens",
        existingDescription: "Token-Werte werden nach der Erstellung nicht mehr angezeigt. Es werden nur Metadaten angezeigt.",
        loadingTokens: "Tokens werden geladen…",
        emptyState: "Noch keine API-Tokens erstellt.",
        tableHeaders: { name: "Name", created: "Erstellt", lastUsed: "Zuletzt verwendet", expires: "Läuft ab", actions: "Aktionen" },
        revoke: "Widerrufen",
        revoked: "Widerrufen",
        expired: "Abgelaufen"
      }
    },
    "login.title": "Bei Pulse anmelden",
    "login.subtitle": "Geben Sie Ihre Anmeldedaten ein, um fortzufahren",
    "login.submit": "Anmelden",
    "dashboard.title": "Übersicht"
  },
  zh: {
    nav: {
      dashboard: "仪表盘",
      monitors: "监控",
      settings: "设置",
      logout: "退出",
      home: "Pulse — 首页"
    },
    common: {
      save: "保存",
      cancel: "取消",
      delete: "删除",
      confirm: "确认",
      retry: "重试",
      apply: "应用",
      back: "返回",
      loading: "加载中...",
      error: "出了点问题",
      copy: "复制",
      copied: "已复制！",
      remove: "移除",
      replace: "替换",
      add: "添加",
      none: "无",
      never: "从未",
      na: "不适用",
      active: "活跃",
      paused: "已暂停",
      saving: "保存中…",
      creating: "创建中…",
      deleting: "删除中…",
      revoking: "撤销中…",
      pageOf: "第 {page} 页，共 {totalPages} 页",
      previous: "上一页",
      next: "下一页"
    },
    connection: {
      live: "已连接",
      reconnecting: "重新连接中…",
      paused: "实时更新已暂停",
      statusLive: "连接状态：已连接",
      statusPaused: "连接状态：实时更新已暂停"
    },
    settings: {
      title: "设置",
      description: "管理您的账户和 API 访问。",
      language: { title: "语言", label: "显示语言" },
      tokens: {
        title: "API 令牌",
        description: "API 令牌提供对 Pulse API 的编程访问。用于 CI/CD 管道、自动化脚本和外部集成。",
        createTitle: "创建新令牌",
        createLabel: "令牌名称",
        createPlaceholder: "例如 ci-deploy, grafana-read",
        createButton: "创建令牌",
        existingTitle: "现有令牌",
        existingDescription: "令牌值在创建后不再显示。仅显示元数据。",
        loadingTokens: "加载令牌中…",
        emptyState: "尚未创建 API 令牌。",
        tableHeaders: { name: "名称", created: "创建时间", lastUsed: "最近使用", expires: "过期时间", actions: "操作" },
        revoke: "撤销",
        revoked: "已撤销",
        expired: "已过期"
      }
    },
    "login.title": "登录 Pulse",
    "login.subtitle": "输入您的凭据以继续",
    "login.submit": "登录",
    "dashboard.title": "仪表盘"
  },
  ja: {
    nav: {
      dashboard: "ダッシュボード",
      monitors: "モニター",
      settings: "設定",
      logout: "ログアウト",
      home: "Pulse — ホーム"
    },
    common: {
      save: "保存",
      cancel: "キャンセル",
      delete: "削除",
      confirm: "確認",
      retry: "再試行",
      apply: "適用",
      back: "戻る",
      loading: "読み込み中...",
      error: "エラーが発生しました",
      copy: "コピー",
      copied: "コピーしました！",
      remove: "削除",
      replace: "置換",
      add: "追加",
      none: "なし",
      never: "なし",
      na: "該当なし",
      active: "アクティブ",
      paused: "一時停止",
      saving: "保存中…",
      creating: "作成中…",
      deleting: "削除中…",
      revoking: "取り消し中…",
      pageOf: "{totalPages} ページ中 {page} ページ",
      previous: "前へ",
      next: "次へ"
    },
    connection: {
      live: "接続中",
      reconnecting: "再接続中…",
      paused: "リアルタイム更新を一時停止中",
      statusLive: "接続状態：接続中",
      statusPaused: "接続状態：リアルタイム更新を一時停止中"
    },
    settings: {
      title: "設定",
      description: "アカウントと API アクセスを管理します。",
      language: { title: "言語", label: "表示言語" },
      tokens: {
        title: "API トークン",
        description: "API トークンは Pulse API へのプログラムによるアクセスを提供します。CI/CD パイプライン、自動化スクリプト、外部連携に使用してください。",
        createTitle: "新しいトークンを作成",
        createLabel: "トークン名",
        createPlaceholder: "例: ci-deploy, grafana-read",
        createButton: "トークンを作成",
        existingTitle: "既存のトークン",
        existingDescription: "トークンの値は作成後に表示されません。メタデータのみ表示されます。",
        loadingTokens: "トークンを読み込み中…",
        emptyState: "API トークンはまだ作成されていません。",
        tableHeaders: { name: "名前", created: "作成日", lastUsed: "最終使用", expires: "有効期限", actions: "操作" },
        revoke: "取り消し",
        revoked: "取り消し済み",
        expired: "期限切れ"
      }
    },
    "login.title": "Pulse にログイン",
    "login.subtitle": "続行するには認証情報を入力してください",
    "login.submit": "ログイン",
    "dashboard.title": "ダッシュボード"
  },
  ko: {
    nav: {
      dashboard: "대시보드",
      monitors: "모니터",
      settings: "설정",
      logout: "로그아웃",
      home: "Pulse — 홈"
    },
    common: {
      save: "저장",
      cancel: "취소",
      delete: "삭제",
      confirm: "확인",
      retry: "재시도",
      apply: "적용",
      back: "뒤로",
      loading: "로딩 중...",
      error: "문제가 발생했습니다",
      copy: "복사",
      copied: "복사됨!",
      remove: "제거",
      replace: "교체",
      add: "추가",
      none: "없음",
      never: "없음",
      na: "해당 없음",
      active: "활성",
      paused: "일시 중지",
      saving: "저장 중…",
      creating: "생성 중…",
      deleting: "삭제 중…",
      revoking: "취소 중…",
      pageOf: "{totalPages} 페이지 중 {page} 페이지",
      previous: "이전",
      next: "다음"
    },
    connection: {
      live: "연결됨",
      reconnecting: "재연결 중…",
      paused: "실시간 업데이트 일시 중지",
      statusLive: "연결 상태: 연결됨",
      statusPaused: "연결 상태: 실시간 업데이트 일시 중지"
    },
    settings: {
      title: "설정",
      description: "계정 및 API 접근을 관리합니다.",
      language: { title: "언어", label: "표시 언어" },
      tokens: {
        title: "API 토큰",
        description: "API 토큰은 Pulse API에 대한 프로그래밍 방식 접근을 제공합니다. CI/CD 파이프라인, 자동화 스크립트 및 외부 통합에 사용하세요.",
        createTitle: "새 토큰 생성",
        createLabel: "토큰 이름",
        createPlaceholder: "예: ci-deploy, grafana-read",
        createButton: "토큰 생성",
        existingTitle: "기존 토큰",
        existingDescription: "토큰 값은 생성 후 표시되지 않습니다. 메타데이터만 표시됩니다.",
        loadingTokens: "토큰 로딩 중…",
        emptyState: "생성된 API 토큰이 없습니다.",
        tableHeaders: { name: "이름", created: "생성일", lastUsed: "마지막 사용", expires: "만료", actions: "작업" },
        revoke: "취소",
        revoked: "취소됨",
        expired: "만료됨"
      }
    },
    "login.title": "Pulse에 로그인",
    "login.subtitle": "계속하려면 자격 증명을 입력하세요",
    "login.submit": "로그인",
    "dashboard.title": "대시보드"
  },
  tr: {
    nav: {
      dashboard: "Kontrol Paneli",
      monitors: "Monitörler",
      settings: "Ayarlar",
      logout: "Çıkış Yap",
      home: "Pulse — Ana Sayfa"
    },
    common: {
      save: "Kaydet",
      cancel: "İptal",
      delete: "Sil",
      confirm: "Onayla",
      retry: "Tekrar Dene",
      apply: "Uygula",
      back: "Geri",
      loading: "Yükleniyor...",
      error: "Bir hata oluştu",
      copy: "Kopyala",
      copied: "Kopyalandı!",
      remove: "Kaldır",
      replace: "Değiştir",
      add: "Ekle",
      none: "Yok",
      never: "Hiçbir zaman",
      na: "Geçersiz",
      active: "Aktif",
      paused: "Duraklatıldı",
      saving: "Kaydediliyor…",
      creating: "Oluşturuluyor…",
      deleting: "Siliniyor…",
      revoking: "İptal ediliyor…",
      pageOf: "Sayfa {page} / {totalPages}",
      previous: "Önceki",
      next: "Sonraki"
    },
    connection: {
      live: "Bağlı",
      reconnecting: "Yeniden bağlanıyor…",
      paused: "Canlı güncellemeler duraklatıldı",
      statusLive: "Bağlantı durumu: bağlı",
      statusPaused: "Bağlantı durumu: canlı güncellemeler duraklatıldı"
    },
    settings: {
      title: "Ayarlar",
      description: "Hesabınızı ve API erişiminizi yönetin.",
      language: { title: "Dil", label: "Görüntüleme dili" },
      tokens: {
        title: "API Tokenları",
        description: "API tokenları Pulse API'ye programatik erişim sağlar. CI/CD hatları, otomasyon betikleri ve harici entegrasyonlar için kullanın.",
        createTitle: "Yeni Token Oluştur",
        createLabel: "Token adı",
        createPlaceholder: "örn. ci-deploy, grafana-read",
        createButton: "Token Oluştur",
        existingTitle: "Mevcut Tokenlar",
        existingDescription: "Token değerleri oluşturulduktan sonra gösterilmez. Yalnızca meta veriler gösterilir.",
        loadingTokens: "Tokenlar yükleniyor…",
        emptyState: "Henüz API tokenı oluşturulmadı.",
        tableHeaders: { name: "Ad", created: "Oluşturulma", lastUsed: "Son Kullanım", expires: "Bitiş", actions: "İşlemler" },
        revoke: "İptal Et",
        revoked: "İptal Edildi",
        expired: "Süresi Doldu"
      }
    },
    "login.title": "Pulse'a Giriş Yap",
    "login.subtitle": "Devam etmek için kimlik bilgilerinizi girin",
    "login.submit": "Giriş Yap",
    "dashboard.title": "Kontrol Paneli"
  },
  it: {
    nav: {
      dashboard: "Pannello di controllo",
      monitors: "Monitor",
      settings: "Impostazioni",
      logout: "Esci",
      home: "Pulse — Home"
    },
    common: {
      save: "Salva",
      cancel: "Annulla",
      delete: "Elimina",
      confirm: "Conferma",
      retry: "Riprova",
      apply: "Applica",
      back: "Indietro",
      loading: "Caricamento...",
      error: "Qualcosa è andato storto",
      copy: "Copia",
      copied: "Copiato!",
      remove: "Rimuovi",
      replace: "Sostituisci",
      add: "Aggiungi",
      none: "Nessuno",
      never: "Mai",
      na: "N/D",
      active: "Attivo",
      paused: "In pausa",
      saving: "Salvataggio…",
      creating: "Creazione…",
      deleting: "Eliminazione…",
      revoking: "Revoca…",
      pageOf: "Pagina {page} di {totalPages}",
      previous: "Precedente",
      next: "Successivo"
    },
    connection: {
      live: "Connesso",
      reconnecting: "Riconnessione…",
      paused: "Aggiornamenti in pausa",
      statusLive: "Stato connessione: connesso",
      statusPaused: "Stato connessione: aggiornamenti in pausa"
    },
    settings: {
      title: "Impostazioni",
      description: "Gestisci il tuo account e l'accesso API.",
      language: { title: "Lingua", label: "Lingua di visualizzazione" },
      tokens: {
        title: "Token API",
        description: "I token API forniscono accesso programmatico all'API di Pulse. Usali per pipeline CI/CD, script di automazione e integrazioni esterne.",
        createTitle: "Crea nuovo token",
        createLabel: "Nome del token",
        createPlaceholder: "es. ci-deploy, grafana-read",
        createButton: "Crea token",
        existingTitle: "Token esistenti",
        existingDescription: "I valori dei token non vengono mostrati dopo la creazione. Vengono visualizzati solo i metadati.",
        loadingTokens: "Caricamento token…",
        emptyState: "Nessun token API creato ancora.",
        tableHeaders: { name: "Nome", created: "Creato", lastUsed: "Ultimo utilizzo", expires: "Scadenza", actions: "Azioni" },
        revoke: "Revoca",
        revoked: "Revocato",
        expired: "Scaduto"
      }
    },
    "login.title": "Accedi a Pulse",
    "login.subtitle": "Inserisci le tue credenziali per continuare",
    "login.submit": "Accedi",
    "dashboard.title": "Pannello di controllo"
  }
};

// Deep merge: overrides only keys present in the override object
function deepMerge(base, override) {
  const result = JSON.parse(JSON.stringify(base));
  for (const [key, value] of Object.entries(override)) {
    if (value && typeof value === 'object' && !Array.isArray(value) && result[key] && typeof result[key] === 'object') {
      result[key] = deepMerge(result[key], value);
    } else {
      result[key] = value;
    }
  }
  return result;
}

for (const [locale, overrides] of Object.entries(translations)) {
  // Build the structured override (handle dot-notation keys for login/dashboard)
  const structured = {};
  for (const [key, value] of Object.entries(overrides)) {
    if (key.includes('.')) {
      const [section, field] = key.split('.');
      if (!structured[section]) structured[section] = {};
      structured[section][field] = value;
    } else {
      structured[key] = value;
    }
  }

  const merged = deepMerge(en, structured);
  const outPath = resolve(localesDir, `${locale}.json`);
  writeFileSync(outPath, JSON.stringify(merged, null, 2) + '\n', 'utf-8');
  console.log(`✓ Generated ${locale}.json`);
}
