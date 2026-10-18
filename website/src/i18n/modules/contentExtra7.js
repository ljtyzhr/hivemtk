function build(K){
  const z={},e={},j={},a={}
  for(const k in K){z[k]=k;e[k]=K[k][0];j[k]=K[k][1];a[k]=K[k][2]}
  return {zh:z,en:e,ja:j,ar:a}
}

export default build({
  // —— /pricing 平价路线 ——
  '平价路线 ·':['Budget paths ·','お手頃ルート ·','مسارات موفّرة ·'],
  '先跑起来，再按需要长大':['get it running first, scale as you need','まず動かし、必要に応じて拡張','شغّل أولاً وتوسّع حسب الحاجة'],
  '按模块单买、不打包套餐：基座与代码永久免费，最小花钱路线只要 ¥2,980/年。以下三条路线的金额均为源码默认定价的算术求和，不是套餐价，也没有必须一次买齐的门槛。':['Buy modules individually — no bundles. The core and code are free forever, and the cheapest paid path is only ¥2,980/year. Amounts below are arithmetic sums of the default source pricing, not bundle prices, and there is no all-or-nothing threshold.','モジュール単位の購入でセットプランなし。基盤とコードは永久無料、最小の有料ルートは年 ¥2,980。以下の金額はすべてソース標準価格の算術合計であり、セット価格でも一括購入の必須ラインでもありません.','شراء الوحدات فردياً — بلا باقات. الأساس والكود مجانيان دائماً، وأرخص مسار مدفوع 2,980 يوان/سنة فقط. المبالغ أدناه مجاميع حسابية لتسعير المصدر الافتراضي لا أسعار باقات، وليس هناك حدّ شراء إلزامي دفعة واحدة.'],
  '¥0 起步线':['¥0 starter path','¥0 スターター','مسار البداية ¥0'],
  '离线自托管':['Offline self-hosted','オフライン自社ホスト','نشر محلي خاص بك'],
  '¥0':['¥0','¥0','¥0'],
  '订阅成本为零':['Zero subscription cost','サブスクリプション費用ゼロ','تكلفة اشتراك صفرية'],
  'AGPL-3.0 全量源码 + 7 个基础模块免费常开，不可下线、不可改价':['Full AGPL-3.0 source + 7 base modules free and always on — cannot be disabled or repriced','AGPL-3.0 完全ソース＋7 基盤モジュールは無料で常時有効。無効化・価格改定は不可','كود AGPL-3.0 كاملاً + 7 وحدات أساسية مجانية دائمة التفعيل — لا تعطيل ولا تغيير سعر'],
  '自带 API Key 接入大模型只统计不计费，或直接跑本地推理栈':['Connect LLMs with your own API key — metered only, no billing — or run the local inference stack','自带 API キーで LLM 接続（計測のみ・課金なし）またはローカル推論スタックを実行','اربط النماذج بمفتاحك — احتساب فقط دون فوترة — أو شغّل حزمة الاستدلال المحلية'],
  '0 元模块（如平台模型算力开通位）自动开通，付费模块锁定并明示':['Zero-price modules (e.g. the platform compute activation slot) auto-enable; priced modules stay locked and clearly labeled','0 円モジュール（プラットフォーム算力の開通枠など）は自動有効化、価格ありモジュールはロックされたまま明示','الوحدات المجانية (مثل تفعيل حساب المنصة) تُفعَّل تلقائياً وتبقى المدفوعة مقفلة ومعلنة'],
  '适合：技术型团队 / 想先跑通流程再决定订阅':['For: technical teams, or validating the workflow before subscribing','対象：技術チーム／購入前にワークフローを検証したいチーム','لفرق تقنية أو من يريد تجربة سير العمل قبل الاشتراك'],
  '¥2,980/年 轻量 AI 线':['¥2,980/year light AI path','¥2,980/年のライト AI ルート','مسار AI خفيف ¥2,980/سنة'],
  '先把 7×24 自动承接跑起来':['get 24/7 auto-answering running first','まず 24 時間自動応答を稼働','شغّل الاستقبال الآلي على مدار الساعة أولاً'],
  '¥2,980/年':['¥2,980/year','¥2,980/年','¥2,980/سنة'],
  'AI 智能体单模块':['AI agent module alone','AI エージェント単体モジュール','وحدة AI agent بالمفرده'],
  'AI 智能体：其依赖全为免费基础模块，无付费依赖补齐':['AI agent: all of its dependencies are free base modules — no paid dependencies to add','AI エージェント：依存はすべて無料の基盤モジュール。有料依存の追加は不要','وكيل AI: كل اعتماداته وحدات أساسية مجانية — بلا اعتمادات مدفوعة تُضاف'],
  '可选 +短链活码 ¥380/年，合计 ¥3,360/年':['Optional + short links & live codes at ¥380/year → ¥3,360/year total','任意：＋ショートリンク・動的コード ¥380/年 → 合計 ¥3,360/年','اختياري + روابط قصيرة وأكواد حية 380 يوان/سنة ← الإجمالي 3,360 يوان/سنة'],
  '模型 Token 成本仍自控：自带 Key 或平台算力按量预付费':['Token costs stay under your control: your own key or prepaid metered platform compute','トークン費用は自己管理：自带キーまたはプラットフォーム算力の従量前払い','تكلفة التوكنات تحت سيطرتك: مفتاحك أو حساب المنصة المدفوع مسبقاً'],
  '适合：中小私域团队 / 高客单咨询承接':['For: small and mid-size private-domain teams handling high-value inquiries','対象：中小規模の私域チーム／高単価商談の受け皿','لفرق النطاق الخاص الصغيرة والمتوسطة الاستقبال الاستشارات مرتفعة القيمة'],
  '¥28,280/年 全量线':['¥28,280/year full path','¥2,980/年のフルルート','مسار كامل ¥28,280/سنة'],
  '16 个年费模块全开':['all 16 yearly modules enabled','16 の年間モジュールすべて有効','تفعيل الوحدات الـ16 السنوية كاملة'],
  '¥28,280/年':['¥28,280/year','¥28,280/年','¥28,280/سنة'],
  '16 个年费模块算术合计':['arithmetic sum of the 16 yearly modules','16 年間モジュールの算術合計','مجمّع حسابي لـ16 وحدة سنوية'],
  '含 AI 销冠双件套（AI 智能体 + AI 销冠 ¥6,960/年）、GEO ¥2,980/年 等全部年费模块':['Includes the AI champion duo (AI agent + AI champion, ¥6,960/year), GEO at ¥2,980/year, and every other yearly module','AI チャンピオン デュオ（AI エージェント＋AI 销冠 ¥6,960/年）、GEO ¥2,980/年 を含む全年間モジュール','يشمل ثنائي AI (وكيل AI + بطل AI بـ6,960 يوان/سنة) وGEO بـ2,980 يوان/سنة وجميع الوحدات السنوية'],
  '平台模型算力 0 元开通，按 token 预付费另计（不计入年费合计）':['Platform model compute is free to enable; prepaid per token is billed separately (not in the annual sum)','プラットフォーム算力は 0 円で有効化。トークン前払いは別途課金（年間合計に不算入）','تفعيل حساب المنصة مجاني؛ والدفع المسبق بالتوكن يُفصل عن المجموع السنوي'],
  '中间档可参考销冠双件套 ¥6,960/年，按业务阶段逐档升配':['A middle tier: the AI champion duo at ¥6,960/year — upgrade tier by tier as the business grows','中間帯は AI 销冠デュオ ¥6,960/年を参考に、事業段階に合わせて段階的に昇格','الطبقة الوسطى: ثنائي بطل AI بـ6,960 يوان/سنة — ارتقِ درجة درجة مع نمو عملك'],
  '适合：多门店 / 集团 / 区域代理全域能力':['For: multi-store businesses, groups and regional agents needing the full capability set','対象：多店舗・グループ・地域代理のフル機能','لمتاجر متعددة ومجموعات ووكلاء مناطق بحاجة للقدرات الكاملة'],
  '基座永远免费':['The base is always free','基盤は永久無料','الأساس مجاني دائماً'],
  '代码 AGPL-3.0 全量开源，7 个基础模块免费且不可下线、不可改价（平台端对基础模块的写操作一律拒绝）——订阅体系动不了你的底线能力。':['The code is fully open source under AGPL-3.0; the 7 base modules are free and cannot be disabled or repriced (the platform rejects all write operations on base modules) — the subscription system cannot take away your baseline capabilities.','コードは AGPL-3.0 で完全オープンソース。7 基盤モジュールは無料かつ無効化・価格改定不可（プラットフォームは基盤モジュールへの書き込みを一律拒否）。サブスクリプション機構でも基盤能力は奪えません.','الكود مفتوح المصدر بالكامل بموجب AGPL-3.0؛ والوحدات السبع مجانية لا تعطَّل ولا يتغير سعرها (المنصة ترفض كل عمليات الكتابة عليها) — نظام الاشتراك لا يجردك من قدراتك الأساسية.'],
  '单买不捆绑':['Buy individually, no bundles','単品購入・バンドルなし','اشتري فردياً بلا باقات'],
  '按模块订阅、不做套餐包（设计决策已明文写入 spec）；依赖闭包只自动补齐免费基础模块，付费依赖必须显式勾选并在确认弹窗列明总价。':['Subscribe per module — no bundles (a design decision written into the spec). The dependency closure only auto-enables free base modules; paid dependencies must be explicitly checked and itemized in the confirmation dialog.','モジュール単位サブスクリプション・セットプランなし（設計判断として spec に明記）。依存閉包は無料の基盤モジュールのみ自動補完し、有料依存は明示チェックと確認ダイアログの明細が必須。','اشتراك لكل وحدة — بلا باقات (قرار تصميم مكتوب في المواصفة). الاعتمادات تُضيف أساسيات مجانية فقط، والمدفوعة تتطلب تحديداً صريحاً وتفصيلاً في نافذة التأكيد.'],
  '模型成本自控':['Model costs stay controllable','モデル費用は自己管理可控','تكلفة النماذج قابلة للضبط'],
  '自带 Key 只统计不计费；平台算力预付费、输入与输出分别计价、余额不足自动阻断或降级——Token 账单可预估、可封顶。':['Own keys are metered, not billed; platform compute is prepaid with input and output priced separately, auto-blocking or degrading on low balance — the token bill is estimable and capping-friendly.','自带キーは計測のみで課金なし。プラットフォーム算力は前払い・入出力別単価・残高不足時は自動ブロックまたは縮退——トークン請求は見積り可能で上限設定しやすい.','مفاتيحك تُحتسب ولا تُفوَّتر؛ وحساب المنصة مدفوع مسبقاً بسعر مستقل للإدخال والإخراج ويُمنع أو يتحوّل تلقائياً عند نقص الرصيد — فاتورة التوكن قابلة للتقدير والحد.'],
  '二开不加钱':['No extra fees for customization','二次開発に追加費用なし','لا رسوم إضافية للتطوير'],
  '订阅的是开通权益与配额，不是代码：源码全量可改，AGPL-3.0 下保持修改后的网络服务开源即可，二次开发不产生额外授权费。':['What you subscribe to is enablement and quota, not the code. The full source can be modified; keep modified network services open source under AGPL-3.0 and there are no extra licensing fees for secondary development.','サブスクリプションの対象は有効化権益とクォータであり、コードではありません。全ソースは改変可能で、AGPL-3.0 に従い改変したネットワークサービスをオープンソースに保てば、二次開発に追加ライセンス費用は発生しません.','الاشتراك يشتري التفعيل والحد لا الكود. الكود كله قابل للتعديل، وأبقِ الخدمات المعدّلة مفتوحة المصدر بموجب AGPL-3.0 فلا رسوم تراخيص للتطوير الثانوي.'],
  '预算有限，最小可用的花钱路线是什么？':['What is the cheapest usable paid path on a tight budget?','予算が限られたとき、最小限の有料ルートは？','ما أرخص مسار مدفوع عملي بميزانية محدودة؟'],  '先走 ¥0 起步线：离线部署 + 7 个基础模块 + 自带 API Key，订阅成本为零；需要 7×24 自动承接时，单买 AI 智能体 ¥2,980/年 即可（其依赖全为免费基础模块，无付费依赖补齐）。其余模块随业务逐个开通，没有必须一次买齐的门槛。':['Start with the ¥0 path: offline deployment + 7 base modules + your own API key — zero subscription cost. When you need 24/7 auto-answering, the AI agent module alone at ¥2,980/year covers it (all its dependencies are free base modules). Enable other modules one by one as the business grows; there is no all-at-once threshold.','まず ¥0 ルート：オフライン導入＋7 基盤モジュール＋自带 API キーでサブスク費用ゼロ。24 時間自動応答が必要になったら AI エージェント単体 ¥2,980/年（依存はすべて無料基盤）。他モジュールは事業に合わせて順次購入可で、一括必須ラインはありません.','ابدأ بمسار ¥0: نشر محلي + 7 وحدات أساسية + مفتاحك — تكلفة اشتراك صفرية. وعند الحاجة لاستقبال آلي 24/7 يكفي وكيل AI بـ2,980 يوان/سنة (اعتماداداته مجانية كلها). وفعّل البقية تدريجياً بلا حدّ إلزامي.'],
  '口径：金额取自源码内置默认定价（user-server/internal/model/module_bindings.go，2026-10-18 核对）；全量线为 16 个年费模块的算术求和（非套餐价），平台模型算力按量预付费不含在内。':['Basis: amounts come from the default pricing built into the source (user-server/internal/model/module_bindings.go, verified 2026-10-18). The full path is the arithmetic sum of the 16 yearly modules (not a bundle price); prepaid platform model compute is excluded.','基準：金額はソース内蔵の標準価格（user-server/internal/model/module_bindings.go、2026-10-18 照合）。フルルートは 16 年間モジュールの算術合計（セット価格ではなく）、プラットフォーム算力の従量前払いは含みません.','الأسس: المبالغ من التسعير الافتراضي في المصدر (module_bindings.go، تم التحقق 2026-10-18). المسار الكامل مجموع حسابي لـ16 وحدة سنوية (لا سعر باقة)، ويستثني الدفع المسبق لحساب المنصة.'],
})
