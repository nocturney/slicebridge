# Privacy / פרטיות

_Last reviewed: 25 October 2026. Scope: public website, the desktop source at SliceBridge 0.4.0-beta.4.dev.28, and the optional MakerWorld browser-extension source. This is a code-level disclosure, **not** an independent network-traffic audit or certification of third-party slicers._

## English

### Website and support

The [SliceBridge website](https://nocturney.github.io/slicebridge/) is a static GitHub Pages site. It does not offer accounts, 3MF uploads, web forms, or intentionally installed analytics cookies, tracking pixels or analytics scripts. GitHub may process IP addresses, request data and hosting logs under [GitHub's Privacy Statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement).

GitHub issues and pull requests are normally public. Do not upload customer models, personal data, passwords or tokens. For security reports, use [private vulnerability reporting](../../.github/SECURITY.md).

### Desktop application

**Local project conversion:** SliceBridge reads 3MF projects from paths you select, produces separate output files on your computer and opens target slicers only when requested. The conversion code does not upload 3MF files to a SliceBridge server. Physical printer, nozzle and material choices and desktop preferences are stored locally in the user's application configuration.

**Network features are optional and separate from conversion:**

- **Check for updates:** clicking the update-check control requests release metadata from `api.github.com` for `nocturney/slicebridge`. The request includes a SliceBridge application-version User-Agent. The app does not automatically download or install updates. If the repository becomes private and an existing GitHub CLI is available, a fallback may use **that CLI's existing GitHub authentication** to request the same release metadata; the app does not ask for or store the CLI token.
- **MakerWorld integration (Developer Mode only):** browser-link setup and importing are disabled by default. After the user enables Developer Mode, installs the optional extension and chooses a mode, the extension runs only on MakerWorld pages. It stores the `off` / `ask` / `always` preference in **browser-local extension storage**. For a supported link that the user chooses to open with SliceBridge, the extension passes a MakerWorld/Bambu download URL through `slicebridge://`; the desktop app downloads the 3MF directly over HTTPS from allowlisted MakerWorld/Bambu hosts into `~/Downloads/SliceBridge`. Download URLs can contain short-lived link parameters. The app does not copy browser login cookies into the request and avoids displaying or logging the raw signed URL.
- **External pages:** explicitly opening release pages, source links or browser setup pages delegates to the user's browser. Those websites have their own privacy practices.

Turning off the optional extension's interception mode restores ordinary MakerWorld link behavior. Disabling Developer Mode blocks the native browser-import handler. Downloaded and converted local files are not deleted automatically; users manage their own copies.

**Limits of this review:** these statements describe the inspected application and extension source paths, not measured outbound traffic for every released binary, operating system, installed slicer or browser. The third-party websites and slicers that users choose to open are outside SliceBridge's control. We have not independently verified an absence of all network activity or third-party telemetry.

## עברית

### אתר ותמיכה

[אתר SliceBridge](https://nocturney.github.io/slicebridge/) הוא אתר סטטי ב־GitHub Pages. אין בו חשבונות, העלאת קובצי 3MF, טפסים מקוונים או כלי אנליטיקה ומעקב ש־SliceBridge התקין ביוזמתו. GitHub עשויה לעבד כתובות IP, בקשות ויומני אירוח לפי [מדיניות הפרטיות שלה](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement).

Issues ו־Pull Requests ב־GitHub גלויים בדרך כלל לציבור. אין להעלות קובצי לקוחות, פרטים אישיים, סיסמאות או טוקנים. על פרצות אבטחה יש לדווח דרך [ערוץ דיווח פרטי](../../.github/SECURITY.md).

### תוכנת המחשב

**המרה מקומית:** SliceBridge קוראת קובצי 3MF שהמשתמש בוחר, יוצרת קובצי פלט נפרדים במחשב ופותחת סלייסרים לפי פעולה מפורשת. קוד ההמרה אינו מעלה את קובצי המודל לשרת של SliceBridge. העדפות המדפסות, הדיזות והחומרים נשמרות בהגדרות המקומיות של המשתמש. התוכנה עשויה לקרוא מטא־דאטה מקומית של מקור ההורדה (ב־macOS באמצעות `mdls`, וב־Windows באמצעות `Zone.Identifier`). המרה ל־PrusaSlicer מפעילה רכיב עזר מקומי המגיע עם התוכנה; פעולות אלו אינן מעלות את קובץ ה־3MF לשרת של SliceBridge.

**פעולות רשת אופציונליות, הנפרדות מההמרה:**

- **בדיקת עדכונים:** רק לאחר לחיצה על בדיקת עדכונים, התוכנה מבקשת מידע על גרסאות של `nocturney/slicebridge` מ־`api.github.com`. הבקשה כוללת מזהה User-Agent עם גרסת התוכנה. אין הורדה או התקנה אוטומטית של עדכונים. אם בקשת הגרסאות תחזיר HTTP 404 (למשל כשהמאגר יהפוך לפרטי), ייתכן שייעשה שימוש בהתחברות קיימת של GitHub CLI לצורך בדיקת הגרסאות בלבד; התוכנה אינה מבקשת או שומרת את הטוקן שלו.
- **חיבור MakerWorld (מצב מפתחים בלבד):** המטפל בייבוא אינו פעיל עד להפעלה מפורשת של מצב מפתחים; ב־macOS פרוטוקול `slicebridge://` עשוי להירשם עם חבילת התוכנה גם כשהייבוא חסום. לאחר הפעלה מפורשת, התקנת התוסף ובחירת אופן הפעולה, התוסף פועל רק בדפי MakerWorld ושומר את הבחירה `off` / `ask` / `always` באחסון המקומי של התוסף בדפדפן. כאשר המשתמש בוחר לפתוח קישור נתמך דרך SliceBridge, מועברת כתובת הורדה באמצעות `slicebridge://`; התוכנה מורידה את קובץ ה־3MF ישירות ב־HTTPS מדומיינים מאושרים של MakerWorld/Bambu לתיקייה `~/Downloads/SliceBridge`. הכתובת עשויה להכיל פרמטרים זמניים. התוכנה אינה מעתיקה עוגיות התחברות מהדפדפן ומנסה להימנע מהצגה או מתיעוד של כתובת ההורדה החתומה במלואה. עם זאת, הכתובת עשויה להישמר בהיסטוריית הדפדפן או להופיע בפרמטרים של תהליך ההפעלה במערכת ההפעלה; יש להתייחס אליה כמידע רגיש.
- **אתרים חיצוניים:** פתיחת דפי גרסאות, תמיכה או התקנת תוסף לפי בקשת המשתמש מתבצעת בדפדפן וכפופה למדיניות האתרים שאליהם נכנסים.

אפשר להחזיר את התוסף למצב כבוי. ביטול מצב מפתחים חוסם את מסלול הייבוא בתוכנה. קבצים מקומיים שיובאו או הומרו נשארים במחשב עד שהמשתמש מוחק אותם.

**גבולות הבדיקה:** התיאור מבוסס על סקירת מסלולי הקוד של התוכנה והתוסף, ולא על ניטור עצמאי של כל תעבורת הרשת בכל מערכת הפעלה, גרסה, דפדפן או סלייסר. אין כאן אישור גורף להיעדר תעבורה או מעקב של תוכנות ואתרים חיצוניים.
