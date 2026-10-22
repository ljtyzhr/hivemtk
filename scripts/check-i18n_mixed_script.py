#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""检查多语言词条里的「串味」：非拉丁语言里混进的英文词/短语、别国文字残留。

为什么要有这个门：新手引导的文案是**逐页手写**的，一次要写 9 种语言。
人工校对 9 份长文本里有没有混进中文/英文，是靠眼睛做不到的事——
本轮实测 ja 版里混进了「BQUICK テナントコード」「organizing ること」
「再起動状態をimation しているなら実際に落ちた){\"previous\":\"ので」，
ar 版里混进了「这三」「最新 part」「只有在」「确定的」「threw استثناءً」
「لا Lent affects」。这些句子读起来像人话，肉眼扫过去完全看不出来，
但对用户就是「这页说明是坏的」。

口径（刻意收窄，只抓真串味，不误伤技术术语）：
  - **非拉丁语言**（zh/ja/ar）里出现**连续 2 个以上的拉丁词** ⇒ 串味。
    技术术语是单个词或已知的连写形式（X-Bridge-Token / llms.txt / top-k），
    连续多个拉丁词一定是把英文句子塞进来了。
  - **非拉丁语言**里出现单个拉丁词，且该词在英文语言包的引导词条里
    以**普通英文单词**（而非专有名词）出现过 ⇒ 可疑，报出来供人确认。
  - **拉丁语言**（es/fr/de/ru/pt）里出现中文/日文假名 ⇒ 串味。
  - **ja** 里出现简体特有字形 ⇒ 串味。

用法：
    python3 scripts/check-i18n_mixed_script.py            # 查 locales 全量
    python3 scripts/check-i18n_mixed_script.py <file.json> # 查单个文件
"""
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LOCALES_DIR = os.path.join(ROOT, "user-web", "src", "i18n", "locales")

NON_LATIN = ("zh", "ja", "ar")
LATIN = ("en", "es", "fr", "de", "ru", "pt")

# 单个拉丁词也可疑的：英文普通词（不是产品名/技术缩写）
SUSPECT_WORDS = {
    "the", "and", "a", "an", "is", "are", "was", "were", "be", "been", "being",
    "to", "of", "in", "on", "at", "for", "with", "by", "from", "as", "that",
    "this", "these", "those", "it", "its", "you", "your", "we", "our", "they",
    "them", "their", "he", "she", "his", "her", "not", "no", "so", "than",
    "then", "when", "while", "which", "who", "whom", "what", "how", "why",
    "can", "could", "should", "would", "will", "may", "might", "must", "do",
    "does", "did", "have", "has", "had", "if", "or", "but", "because", "about",
    "after", "before", "between", "into", "over", "under", "up", "down", "out",
    "only", "also", "even", "still", "already", "always", "never", "more",
    "most", "much", "many", "some", "any", "each", "both", "all", "one", "two",
    "three", "four", "five", "six", "seven", "eight", "nine", "ten", "first",
    "last", "next", "other", "another", "same", "different", "new", "old",
    "good", "bad", "better", "worse", "best", "worst", "real", "true", "false",
    "right", "left", "back", "down", "away", "here", "there", "now", "then",
    "see", "seen", "show", "shows", "shown", "get", "gets", "got", "make",
    "makes", "made", "take", "takes", "taken", "give", "gives", "given", "use",
    "used", "uses", "using", "set", "sets", "put", "add", "adds", "run", "runs",
    "running", "keep", "keeps", "kept", "find", "finds", "found", "need",
    "needs", "needed", "want", "wants", "work", "works", "working", "call",
    "calls", "calling", "know", "knows", "say", "says", "tell", "tells", "ask",
    "asks", "read", "reads", "write", "writes", "check", "checks", "pick",
    "picks", "picked", "click", "clicks", "start", "starts", "stop", "stops",
    "change", "changes", "changed", "update", "updates", "updated", "create",
    "creates", "created", "delete", "deletes", "deleted", "edit", "edits",
    "list", "lists", "page", "pages", "content", "contents", "status", "name",
    "names", "type", "types", "value", "values", "count", "counts", "number",
    "source", "sources", "target", "result", "results", "action", "actions",
    "state", "states", "state", "option", "options", "field", "fields",
    "message", "messages", "session", "sessions", "task", "tasks", "job",
    "jobs", "template", "templates", "config", "configs", "configuration",
    "system", "systems", "model", "models", "channel", "channels", "group",
    "groups", "level", "levels", "item", "items", "row", "rows", "column",
    "table", "tables", "form", "forms", "button", "buttons", "step", "steps",
    "day", "days", "week", "weeks", "time", "times", "user", "users", "role",
    "roles", "key", "keys", "id", "ids", "note", "notes", "info", "data",
    "detail", "details", "record", "records", "view", "views", "search",
    "filter", "filters", "sort", "order", "price", "cost", "costs", "total",
    "empty", "error", "errors", "reason", "reasons", "part", "parts", "way",
    "ways", "thing", "things", "case", "cases", "point", "points", "side",
    "sides", "level", "mean", "means", "means", "help", "helps", "need",
    "work", "workable", "organizing", "organised", "organized", "states",
    "state", "imation", "latest", "certain", "certainly", "only", "sure",
    "threw", "throws", "lent", "affects", "affect", "confirm", "confirms",
    "confirming", "come", "comes", "came", "give", "gives", "gave", "take",
    "back", "before", "after", "above", "below", "under", "over", "during",
    "without", "within", "across", "along", "around", "behind", "beyond",
    "despite", "except", "including", "instead", "toward", "towards",
    "upon", "via", "per", "etc", "vs", "e.g", "i.e",
}

# 明确是产品名/技术名，允许出现在任何语言里（大小写不敏感）
ALLOWED_TERMS = {
    "api", "url", "urls", "uri", "llm", "llms", "ai", "id", "ids", "ui", "ux",
    "sse", "websocket", "webhook", "webhooks", "html", "css", "json", "yaml",
    "yml", "csv", "md", "markdown", "pdf", "sql", "ip", "tcp", "udp", "http",
    "https", "ttl", "p50", "p95", "p99", "rtf", "seo", "txt", "hash", "payload",
    "sdk", "cdn", "oss", "cos", "s3", "rbac", "mcp", "rag", "sms", "email",
    "smtp", "imap", "pop3", "tag", "tags", "bot", "bots", "qr", "otp", "mfa",
    "2fa", "csv", "utf", "gb", "mb", "kb", "tb", "ms", "px", "pt", "ocr",
    "kpi", "pv", "uv", "gmv", "roi", "sov", "vis", "faq", "crm", "erp", "orm",
    "app", "ios", "android", "h5", "h1", "h2", "h3", "h4", "uuid", "sla",
    "slo", "rto", "rpo", "ipv4", "ipv6", "mac", "pc", "qrcode", "ga", "utm",
    "cp", "ctr", "cvr", "acv", "arpu", "ltv", "aov", "cpn", "d7", "d8",
    "webhook-url", "agent", "agents", "prompt", "prompts", "sop", "sops",
    "playground", "brain", "asset", "assets", "bundle", "bundles", "pack",
    "packs", "cohort", "rfm", "csat", "nps", "dkim", "spf", "dmarc",
    "starttls", "ssl", "tls", "sendgrid", "ses", "smtp-server",
    "whatsapp", "telegram", "facebook", "instagram", "douyin", "kuaishou",
    "xiaohongshu", "xianyu", "tiktok", "wechat", "wecom", "qq", "feishu",
    "dingtalk", "alipay", "wechatpay", "appid", "appsecret", "botfather",
    "botsecret", "baidu", "bing", "google", "zhihu", "llms.txt", "robots",
    "robots.txt", "sitemap", "describedby", "x-robots-tag", "x-bridge-token",
    "bridge_token", "hivemtk", "hive-mtk", "schema", "jsonl", "top-k",
    "topk", "minimax", "openai", "anthropic", "claude", "gpt", "qwen",
    "deepseek", "gemini", "llama", "redis", "mysql", "postgres", "postgresql",
    "nginx", "docker", "kubernetes", "k8s", "jwt", "oauth", "openid", "saml",
    "sso", "cron", "ipv6", "png", "jpg", "jpeg", "gif", "svg", "webp",
    "avif", "mrr", "ndcg", "recall", "precision", "eeat", "telegram",
    "curl", "async", "job_no", "openapi", "last_sync_at", "watermark",
    "geo", "llms.txt", "notion", "polling", "webmaster", "blockquote", "www",
    "tencent cloud", "alibaba cloud", "huawei cloud", "google cloud",
    "cohort retention", "behaviour path", "behavior path", "bad case",
    "access token", "title/content/ref", "provider/model", "a/b/c",
    "created/active/completed/failed/stopped", "llms.txt v2", "changes",
    "actions", "case", "bad", "after", "lets you compose one by hand",
    "you", "one", "by", "wants", "cloud",
    "tencent cloud", "alibaba cloud", "huawei cloud", "cohort retention",
    "behaviour path", "behavior path", "bad case", "access token",
    "title/content/ref", "provider/model", "a/b/c", "google cloud",
    "created/active/completed/failed/stopped", "lets you compose one by hand",
    "llms.txt v2",
    "provider", "model", "workflow", "docx", "documents", "title", "content",
    "ref", "template", "phone", "business", "number", "crawled", "hit",
    "reliably", "go", "v2", "a", "b", "c", "start", "taken",
    "hive", "mtk", "d7-gate", "x-bridge", "sse-stream", "brainstorm",
    "a/b", "ab-test", "coolify", "lta", "ltc",
}

# 简体特有字形（日在日文中不出现）——只查 ja
SIMPLIFIED_MARKERS = set(
    # 只收「日语里绝对不存在的简体字形」。像 //额/据// 这类日文本
    # 就有的汉字不能进这张表——进表就会把正常的日文判成串味。
    "这说过还发开关东车马长门问间无业时会队阶阳阴双变号叶听员读击"
    "剑剧划别劝办动务劳势医单卖卫却厌叙吓吕吗呐"
    "坠垒垫壳够头夸夹夺奖妆妇妈娄娇娱婴孙学"
    "宁宝宠审宫宽对寻导寿将尔尘尸尽层属岁岛岭币师带帮广庄庆库应庙"
    "废开异张弥弯弹强归录彻忆忧怀态怜总恋恳恶恼悦悬惊惧惨惩惯愿"
    "懒戏战户执扩扫扬扰抚抛抢护报担拟拥拦拨择挂挚挠挡挣挤挥损换捣据"
    "掷掺摊撑攒敌斋斗斩无旧时旷显晋晒晓晕暂术机杀杂权条极构枪"
    "枫柜标栋栏树样档桥梦检楼欢欧残殡毁毕毙毡气汉污汤沟没沥沦洁洒测"
    "济浏浑浓涂涌涛润涨涩渊渐渗温湾湿滚滞满滤滥滨滩潜灭灯灵灾炉"
    "炼烂烛烟烦烧烫热爱牵独狮狱猎猫献环现玛玮电画畅疗疮疯痒瘫皱盐监"
    "盖盗盘睁矫矿码砖础硅硕确碍礼祸离种积称秘税稳穷窃窜竖竞笋笔筑"
    "筛签简类粪粮紧红纪约级纯纸纹线练组细织终绍经绑结绕给络绝统绢绣"
    "继绩续绳维绵综绿编缘缩网罗罚罢羁翘耻聂聋职联聪肃肠肤肾肿胀胁胆"
    "胜脉脏脐脑脓脚脱脸腊腾舆舰舱艰艳艺节芦苏苹茎茧荐荡荣药获莹萝"
    "萤营萧蒋蓝虏虑虚虽蚁蜡蝇蝉补衬袄装裤见规览觉誉认订讲论设访"
    "证评识译试话该详语误说请读课谁调谈谢谨谱贝贞负贡财责贤败账货质"
    "贩贪贫购贯贱贴贵贷贸费贺贼资赋赌赏赔赖赚赛赞赠赢赣赵赶趋跃践踪"
    "车轨转轮软轰轻载较辅辆辈辉输辖辗辙辞辩边达迁过迈运还进远违连迟"
    "适选逊递逻遗邓邮邹邻郁郑酝释里鉴针钉钓钟钢钥钦钩钮钱钳钻铁铃"
    "铅银铜铝铺链销锁锋错锡镇镜长闭闯闲闷闻阀阁阅阵阶际陆陈险随隐"
    "难雏雾霉静韦韧页顶项顺须顽顾顿预领颇颈频颓题颜额风飘飞饥饭饮饰"
    "饱饲饶饼饿馆马驱驳驴驶驻驼驾驿骂骄骆验骏骑骗骚骤鱼鲁鲜鸟鸡鸣"
    "鸥鸦鸭鸽鹅鹉鹏鹤麦黄齐齿龄龙"
)

#: 短语级允许表：整段短语命中就放行（如 "Tencent Cloud"、"Cohort retention"）。
ALLOWED_PHRASES = {
    "tencent cloud", "alibaba cloud", "huawei cloud", "google cloud",
    "cohort retention", "behaviour path", "behavior path", "bad case",
    "access token", "title/content/ref", "provider/model", "a/b/c",
    "created/active/completed/failed/stopped", "llms.txt v2",
    "lets you compose one by hand",
}

LATIN_RE = re.compile(r"[A-Za-z][A-Za-z0-9_.\-/]*")
CJK_RE = re.compile(r"[\u4e00-\u9fff]")
KANA_RE = re.compile(r"[\u3040-\u30ff]")
# 连续 2 个以上的拉丁词（允许中间有空格/连字符/斜杠）
LATIN_PHRASE_RE = re.compile(
    r"[A-Za-z][A-Za-z0-9_.\-]*(?:[\s/]+[A-Za-z][A-Za-z0-9_.\-]*)+"
)


def collect_latins(text):
    out = []
    for m in LATIN_RE.finditer(text):
        w = m.group(0).strip(".-_/")
        if w:
            out.append(w)
    return out


def check_text(lang, text, en_words):
    problems = []
    if lang in NON_LATIN:
        # 1) 连续 2 个以上拉丁词 → 一定是把英文句子塞进来了
        for m in LATIN_PHRASE_RE.finditer(text):
            phrase = " ".join(m.group(0).split())
            if phrase.lower() in ALLOWED_PHRASES:
                continue
            toks = [t.strip(".-_/").lower() for t in phrase.split() if t.strip(".-_/")]
            # 全部都是已知术语 → 放行（如 "X-Bridge-Token bridge_token"）
            if toks and all(t in ALLOWED_TERMS for t in toks):
                continue
            problems.append(("latin-phrase", lang, phrase))
        # 2) 单个可疑拉丁词（普通英文单词）
        for w in set(collect_latins(text)):
            base = w.lower().strip(".-_/")
            if not base or base in ALLOWED_TERMS:
                continue
            if base in SUSPECT_WORDS:
                problems.append(("latin-word", lang, w))
        # 3) ja 里的简体字
        if lang == "ja":
            for ch in set(text):
                if ch in SIMPLIFIED_MARKERS:
                    problems.append(("simplified", lang, ch))
    else:
        # 拉丁语言：查中文/假名残留
        for m in set(CJK_RE.findall(text)):
            problems.append(("cjk", lang, m))
        for m in set(KANA_RE.findall(text)):
            problems.append(("kana", lang, m))
    return problems


def walk(node, path=""):
    if isinstance(node, dict):
        for k, v in node.items():
            yield from walk(v, f"{path}.{k}" if path else k)
    elif isinstance(node, str):
        yield path, node


def en_common_words():
    """英文引导词条里出现过的「普通英文单词」（小写、长度>2）。"""
    data = json.load(open(os.path.join(LOCALES_DIR, "en.json"), encoding="utf-8"))
    words = set()
    for _k, v in walk(data.get("setup", {}).get("onboarding", {})):
        for w in collect_latins(v):
            base = w.lower().strip(".-_/")
            if len(base) > 2 and base.isalpha():
                words.add(base)
    return words


def main():
    files = sys.argv[1:]
    if not files:
        files = [os.path.join(LOCALES_DIR, f"{l}.json") for l in
                 ("zh", "en", "ja", "ar", "es", "fr", "de", "ru", "pt")]
    en_words = en_common_words()
    total = 0
    for f in files:
        lang = os.path.splitext(os.path.basename(f))[0]
        data = json.load(open(f, encoding="utf-8"))
        for key, val in walk(data):
            if not key.startswith("setup.onboarding.page."):
                continue
            for kind, lg, token in check_text(lang, val, en_words):
                total += 1
                print(f"{os.path.basename(f)} [{kind}] {key} :: {token!r}")
    print(f"\n共 {total} 处待查")
    return 1 if total else 0


if __name__ == "__main__":
    sys.exit(main())
