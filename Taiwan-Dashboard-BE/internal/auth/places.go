package auth

import "strings"

var taiwanPlaces = map[string]string{
	"taipei": "臺北", "newtaipei": "新北", "taoyuan": "桃園", "taichung": "臺中", "tainan": "臺南",
	"kaohsiung": "高雄", "keelung": "基隆", "hsinchu": "新竹", "chiayi": "嘉義", "miaoli": "苗栗",
	"changhua": "彰化", "nantou": "南投", "yunlin": "雲林", "pingtung": "屏東", "yilan": "宜蘭",
	"hualien": "花蓮", "taitung": "臺東", "penghu": "澎湖", "kinmen": "金門", "lienchiang": "連江",
	"matsu": "連江",

	"banqiao": "新北板橋", "sanchong": "新北三重", "zhonghe": "新北中和", "yonghe": "新北永和",
	"xinzhuang": "新北新莊", "xindian": "新北新店", "tucheng": "新北土城", "luzhou": "新北蘆洲",
	"shulin": "新北樹林", "xizhi": "新北汐止", "sanxia": "新北三峽", "tamsui": "新北淡水",
	"danshui": "新北淡水", "linkou": "新北林口", "yingge": "新北鶯歌", "wugu": "新北五股",
	"taishan": "新北泰山",
	"zhongli": "桃園中壢", "pingzhen": "桃園平鎮", "bade": "桃園八德", "yangmei": "桃園楊梅",
	"guishan": "桃園龜山", "luzhu": "桃園蘆竹", "dayuan": "桃園大園", "longtan": "桃園龍潭",
	"daxi":   "桃園大溪",
	"zhubei": "新竹竹北", "zhudong": "新竹竹東", "hukou": "新竹湖口",
	"toufen": "苗栗頭份", "zhunan": "苗栗竹南",
	"fengyuan": "臺中豐原", "dali": "臺中大里", "taiping": "臺中太平", "wufeng": "臺中霧峰",
	"shalu": "臺中沙鹿", "qingshui": "臺中清水", "tanzi": "臺中潭子", "wuri": "臺中烏日",
	"yuanlin": "彰化員林", "lukang": "彰化鹿港", "douliu": "雲林斗六", "huwei": "雲林虎尾",
	"taibao": "嘉義太保", "puzi": "嘉義朴子", "yongkang": "臺南永康", "xinying": "臺南新營",
	"rende": "臺南仁德", "fengshan": "高雄鳳山", "gangshan": "高雄岡山", "zuoying": "高雄左營",
	"nanzi": "高雄楠梓", "chaozhou": "屏東潮州", "donggang": "屏東東港", "luodong": "宜蘭羅東",
	"magong": "澎湖馬公", "jincheng": "金門金城", "nangan": "連江南竿",
}

func taiwanPlace(name string) string {
	k := strings.ToLower(strings.TrimSpace(name))
	for _, suffix := range []string{" city", " county", " district", " township", " city government"} {
		k = strings.TrimSuffix(k, suffix)
	}
	k = strings.NewReplacer(" ", "", "-", "", "'", "").Replace(k)
	return taiwanPlaces[k]
}
