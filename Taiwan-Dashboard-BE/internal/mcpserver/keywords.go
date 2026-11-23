package mcpserver

import (
	"sort"
	"strings"
)

var englishTerms = map[string][]string{
	"air quality": {"空氣品質", "空品"}, "air pollution": {"空氣品質", "空品"}, "aqi": {"空氣品質"},
	"pm2.5": {"pm2.5", "細懸浮微粒"}, "pm25": {"pm2.5", "細懸浮微粒"}, "uv": {"紫外線"}, "ultraviolet": {"紫外線"},
	"weather": {"天氣", "氣象"}, "temperature": {"氣溫", "溫度"}, "rain": {"雨量", "降雨"}, "rainfall": {"雨量", "降雨"},
	"forecast": {"預報"}, "wind": {"風"}, "typhoon": {"颱風"}, "earthquake": {"地震"}, "quake": {"地震"},
	"warning": {"警報", "特報", "警戒"}, "alert": {"警報", "警戒"}, "disaster": {"災害", "防災"},
	"landslide": {"土石流"}, "debris flow": {"土石流"}, "flood": {"淹水"}, "flooding": {"淹水"},
	"reservoir": {"水庫"}, "water storage": {"水庫", "蓄水"}, "dam": {"水庫"}, "river": {"河川"},
	"water level": {"水位"}, "water quality": {"水質"}, "water outage": {"停水"}, "water": {"水"},
	"power": {"電力", "發電", "用電"}, "electricity": {"電力", "發電", "用電"}, "power outage": {"停電"},
	"blackout": {"停電"}, "outage": {"停電", "停水"}, "renewable": {"再生能源"}, "solar": {"太陽能"},
	"reserve margin": {"備轉容量"}, "power generation": {"發電"}, "power demand": {"用電"},
	"traffic": {"交通", "道路"}, "freeway": {"國道", "高速公路"}, "highway": {"國道", "公路"},
	"accident": {"事故"}, "rail": {"鐵路", "臺鐵"}, "train": {"鐵路", "臺鐵", "列車"}, "railway": {"鐵路", "臺鐵"},
	"bike": {"單車", "youbike"}, "bicycle": {"單車", "youbike"}, "youbike": {"youbike"},
	"flight": {"航班", "航空"}, "airport": {"機場", "航空"}, "aviation": {"航空"},
	"ev": {"充電"}, "charging": {"充電"}, "fuel": {"油價", "加油"}, "gas price": {"油價"}, "gasoline": {"油價", "汽油"},
	"gas station": {"加油站"}, "population": {"人口"}, "birth": {"出生"}, "death": {"死亡"},
	"income": {"所得"}, "economy": {"經濟"}, "housing": {"房價", "不動產"}, "real estate": {"房價", "不動產"},
	"hospital": {"醫院", "醫療"}, "medical": {"醫療"}, "clinic": {"診所", "醫療"}, "long-term care": {"長照"},
	"dengue": {"登革熱"}, "disease": {"傳染病", "疾病"}, "flu": {"類流感"}, "influenza": {"類流感"},
	"food": {"食品"}, "food safety": {"食品"}, "agriculture": {"農產品", "農業"}, "produce": {"農產品"},
	"vegetable": {"蔬菜", "農產品"}, "fruit": {"水果", "農產品"}, "pork": {"毛豬"}, "hog": {"毛豬"},
	"poultry": {"家禽"}, "chicken": {"雞", "家禽"}, "school": {"學校"}, "childcare": {"托育", "托嬰"},
	"tourism": {"觀光"}, "tourist": {"觀光", "景點"}, "hotel": {"旅宿", "旅館"}, "attraction": {"景點"},
	"animal shelter": {"收容所"}, "pet": {"寵物", "收容"}, "adoption": {"認養"}, "fire": {"火災"},
	"shelter": {"避難", "收容"}, "evacuation": {"避難"}, "school closure": {"停班停課"}, "closure": {"停班停課"},
	"waste": {"垃圾", "廢棄物"}, "recycling": {"資源回收"}, "sustainability": {"永續"},
}

func expandQuery(q string) []string {
	q = strings.ToLower(strings.ReplaceAll(q, "台", "臺"))
	terms := map[string]bool{}
	phrases := make([]string, 0, len(englishTerms))
	for k := range englishTerms {
		phrases = append(phrases, k)
	}
	sort.Slice(phrases, func(i, j int) bool { return len(phrases[i]) > len(phrases[j]) })
	rest := " " + q + " "
	for _, p := range phrases {
		if strings.Contains(rest, " "+p+" ") {
			for _, zh := range englishTerms[p] {
				terms[zh] = true
			}
			if strings.Contains(p, " ") {
				rest = strings.ReplaceAll(rest, " "+p+" ", " ")
			}
		}
	}
	for _, w := range strings.Fields(strings.NewReplacer(",", " ", "，", " ", "、", " ").Replace(q)) {
		terms[w] = true
	}
	out := make([]string, 0, len(terms))
	for t := range terms {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
