package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"

	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
)

// 生成只读方法库

const (
	funcFileSuffix     = "_func.go"                                                  // 只读方法后缀
	jsonFileSuffix     = "_json.go"                                                  // json转换后缀
	dataCtrlFileSuffix = "_data_ctrl.go"                                             // 数据控制器后缀
	fileStatement      = "// Code generated by \"shareddata $GOFILE\"; DO NOT EDIT." // 文件声明
)

func main() {
	// 传入参数检查
	if len(os.Args) < 2 {
		log.Println("未传入解析文件")
		os.Exit(2)
	}
	// 文件基本检查
	fileName := strings.TrimSpace(os.Args[1])
	if !strings.HasSuffix(fileName, ".go") {
		log.Printf("指定文件[%s]非go文件\n", fileName)
		os.Exit(2)
	}
	pkgName, sInfoList := AnalysisStruct(fileName)
	// 生成只读方法
	fileName, err := generateFunc(pkgName, sInfoList)
	if err != nil { // 生成失败
		log.Printf("Generate [%s] Failed!\n", err)
	}
	log.Printf("Generate [%s] success!", fileName)

	// 生成json读取文件
	fileName, err = generateJsonReader(pkgName, sInfoList)
	if err != nil { // 生成失败
		log.Printf("Generate [%s] Failed!\n", err)
	}
	log.Printf("Generate [%s] success!", fileName)

	// 生成数据控制器
	fileName, err = generateDataCtrl(pkgName, sInfoList)
	if err != nil { // 生成失败
		log.Printf("Generate [%s] Failed!\n", err)
	}
	log.Printf("Generate [%s] success!", fileName)
}

// AnalysisStruct 解析结构体定义文件
func AnalysisStruct(fileName string) (string, []*structInfo) {
	fSet := token.NewFileSet()
	file, err := parser.ParseFile(fSet, fileName, nil, parser.ParseComments)
	if err != nil {
		log.Panicln(err)
	}

	rootName := strings.ToUpper(file.Name.Name[:1]) + file.Name.Name[1:]
	structInfoList := make([]*structInfo, 0)
	ast.Inspect(file, func(x ast.Node) bool {
		ts, ok := x.(*ast.TypeSpec)
		if !ok || ts.Type == nil {
			return true
		}
		s, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		// 初始化数据结构
		sInfo := new(structInfo)
		sInfo.Name = ts.Name.Name
		sInfo.FiledList = make([]string, 0)
		sInfo.TypeList = make([]string, 0)
		sInfo.keyIndex = -1
		// 循环读取字段
		for _, field := range s.Fields.List {
			kind := reflect.TypeOf(file).Kind()
			if kind == reflect.Struct || kind == reflect.Array || kind == reflect.UnsafePointer || len(field.Names) == 0 {
				continue
			}
			// 追加字段名信息
			sInfo.FiledList = append(sInfo.FiledList, field.Names[0].Name)
			if field.Tag != nil {
				tag := reflect.StructTag(field.Tag.Value[1 : len(field.Tag.Value)-1])
				if tag.Get("key") == "true" { // 主键索引
					if sInfo.keyIndex >= 0 {
						fmt.Println("主键必须唯一!")
					} else {
						sInfo.keyIndex = len(sInfo.FiledList) - 1
					}
				}
			}
			// 字段类型
			var typeNameBuf bytes.Buffer
			err := printer.Fprint(&typeNameBuf, fSet, field.Type)
			if err != nil {
				fmt.Println("获取类型失败:", err)
				return true
			}
			// 追加类型信息
			sInfo.TypeList = append(sInfo.TypeList, typeNameBuf.String())
		}

		if rootName == sInfo.Name && sInfo.keyIndex < 0 {
			fmt.Println("无指定主键，主键自增")
		}
		if len(sInfo.FiledList) > 0 {
			structInfoList = append(structInfoList, sInfo)
		}
		return true
	})
	return file.Name.Name, structInfoList
}

type structInfo struct {
	Name      string   // 结构体名称
	FiledList []string // 字段名list
	TypeList  []string // 字段类型list
	keyIndex  int      // 主键字段索引
}

// 生成只读方法库
func generateFunc(pkgName string, sInfoList []*structInfo) (string, error) {
	fileName := pkgName + funcFileSuffix
	rootName := strings.ToUpper(pkgName[:1]) + pkgName[1:]
	fp, err := os.Create(fileName)
	if err != nil {
		return fileName, fmt.Errorf("generateFunc create file[%s] Err:%s", fileName, err.Error())
	}

	// 自动化生成声明
	_, _ = fmt.Fprintln(fp, fileStatement)
	_, _ = fmt.Fprintf(fp, "package %s\n", pkgName) // 包名
	// 内容生产
	for _, sInfo := range sInfoList {
		// 只读方法
		_, _ = fmt.Fprintf(fp, "\n/*----- %s -----*/\n", sInfo.Name)
		sName := strings.ToLower(sInfo.Name[:1])
		for index, filedName := range sInfo.FiledList {
			fName := strings.ToUpper(filedName[:1]) + filedName[1:]
			typeStr := sInfo.TypeList[index]

			if rootName == sInfo.Name {
				_, _ = fmt.Fprintf(fp, "\nfunc (%s *%s) Get%s() %s {\n", sName, sInfo.Name, fName, typeStr)
			} else {
				_, _ = fmt.Fprintf(fp, "\nfunc (%s %s) Get%s() %s {\n", sName, sInfo.Name, fName, typeStr)
			}
			_, _ = fmt.Fprintf(fp, "\treturn %s.%s\n", sName, filedName)
			_, _ = fmt.Fprintf(fp, "}\n")
		}
	}
	_ = fp.Close()
	return fileName, nil
}

// 生成json映射结构
func generateJsonReader(pkgName string, sInfoList []*structInfo) (string, error) {
	fileName := pkgName + jsonFileSuffix
	rootName := strings.ToUpper(pkgName[:1]) + pkgName[1:]
	fp, err := os.Create(fileName)
	if err != nil {
		return fileName, fmt.Errorf("generateJsonReader create file[%s] Err:%s", pkgName+jsonFileSuffix, err.Error())
	}
	// 自动化生成声明
	_, _ = fmt.Fprintln(fp, fileStatement)
	_, _ = fmt.Fprintf(fp, "package %s\n", pkgName) // 包名
	// json映射生产
	for _, sInfo := range sInfoList {
		// 计算最大长度
		fLen := 0
		tLen := 0
		for index, filedName := range sInfo.FiledList {
			if len(filedName) > fLen {
				fLen = len(filedName)
			}
			tStr := getJsonType(sInfo.TypeList[index])
			if len(tStr) > tLen {
				tLen = len(tStr)
			}
		}
		jsonName := strings.ToLower(sInfo.Name)

		_, _ = fmt.Fprintf(fp, "\n/*----- %s_json -----*/\n", jsonName)
		// 生成结构
		_, _ = fmt.Fprintf(fp, "\ntype _%sJson struct {\n", jsonName) // 结构体对应json映射名称
		for index, filedName := range sInfo.FiledList {
			fName := strings.ToUpper(filedName[:1]) + filedName[1:]
			typeStr := sInfo.TypeList[index]
			_, _ = fmt.Fprintf(fp, "\t%-"+strconv.Itoa(fLen)+"s %-"+strconv.Itoa(tLen)+"s `json:\"%s\"`\n",
				fName, getJsonType(typeStr), strings.ToLower(filedName))
		}
		_, _ = fmt.Fprintf(fp, "}\n")

		// 数据转换方法
		sName := jsonName[:1]
		if rootName == sInfo.Name {
			_, _ = fmt.Fprintf(fp, "\nfunc (%s _%sJson) _convert() *%s {\n", sName, jsonName, sInfo.Name)
		} else {
			_, _ = fmt.Fprintf(fp, "\nfunc (%s _%sJson) _convert() %s {\n", sName, jsonName, sInfo.Name)
		}
		_, _ = fmt.Fprintf(fp, "\tdata := new(%s)\n", sInfo.Name)
		for index, filedName := range sInfo.FiledList {
			fName := strings.ToUpper(filedName[:1]) + filedName[1:]
			typeStr := sInfo.TypeList[index]
			if typeStr != getJsonType(typeStr) {
				if isList(typeStr) {
					_, _ = fmt.Fprintf(fp, "\tdata.%s = make(%s, 0)\n", filedName, typeStr)
					_, _ = fmt.Fprintf(fp, "\tfor _, e := range %s.%s {\n", sName, fName)
					_, _ = fmt.Fprintf(fp, "\t\tdata.%s = append(data.%s, e._convert())\n", filedName, filedName)
					_, _ = fmt.Fprintf(fp, "\t}\n")
				} else {
					_, _ = fmt.Fprintf(fp, "\tdata.%s = %s.%s._convert()\n", filedName, sName, fName)
				}
			} else {
				_, _ = fmt.Fprintf(fp, "\tdata.%s = %s.%s\n", filedName, sName, fName)
			}
		}

		if rootName == sInfo.Name {
			_, _ = fmt.Fprintf(fp, "\treturn data\n")
		} else {
			_, _ = fmt.Fprintf(fp, "\treturn *data\n")
		}
		_, _ = fmt.Fprintf(fp, "}\n")
	}
	_ = fp.Close()
	return fileName, nil
}

func getJsonType(tStr string) string {
	tStr = strings.TrimSpace(tStr)
	switch tStr {
	case "int", "int8", "int16", "int32", "int64",
		"float32", "float64", "string", "bool":
		return tStr
	default:
		if strings.HasPrefix(tStr, "map[") {
			return tStr
		}
		if isList(tStr) {
			return "[]" + getJsonType(tStr[2:])
		}
		return fmt.Sprintf("_%sJson", tStr)
	}
}
func isList(tStr string) bool {
	return strings.HasPrefix(tStr, "[]")
}

// 生成数据控制器
func generateDataCtrl(pkgName string, sInfoList []*structInfo) (string, error) {

	fileName := pkgName + dataCtrlFileSuffix
	// 检查是否拥有包核心数据: struct命名为包名的首字母大写
	rootName := strings.ToUpper(pkgName[:1]) + pkgName[1:]
	rsInfo := getStruct(rootName, sInfoList)
	if rsInfo == nil {
		return fileName, fmt.Errorf("generateDataCtrl Err: missing root struct:%s", rootName)
	}
	fp, err := os.Create(fileName)
	if err != nil {
		return fileName, fmt.Errorf("generateDataCtrl create file[%s] Err:%s", pkgName+jsonFileSuffix, err.Error())
	}
	// 自动化生成声明
	_, _ = fmt.Fprintln(fp, fileStatement)
	_, _ = fmt.Fprintf(fp, "package %s\n", pkgName) // 包名
	// 引用
	_, _ = fmt.Fprintf(fp, "import (\n"+
		"\t\"encoding/json\"\n"+
		"\t\"os\"\n"+
		")\n") // 包名

	_, _ = fmt.Fprintf(fp, "\nconst JsonFile = \"%s.json\"\n", pkgName)
	// 类型定义or声明
	_, _ = fmt.Fprintf(fp, "\ntype DataSet struct {\n")
	_, _ = fmt.Fprintf(fp, "\tset []*%s // 数据集合\n", rootName)
	_, _ = fmt.Fprintf(fp, "\tindexs map[any]int // 索引\n")
	_, _ = fmt.Fprintf(fp, "}\n")
	// 必要接口
	_, _ = fmt.Fprintf(fp, "\nfunc (s *DataSet) Len() int {\n"+
		"\treturn len(s.set)\n"+
		"}\n")
	// 加载数据接口
	_, _ = fmt.Fprintf(fp, "\nfunc (s *DataSet) Load(key any) *%s {\n"+
		"\tif i, ok := s.indexs[key]; ok {\n"+
		"\t\treturn s.set[i]\n"+
		"\t}\n"+
		"\treturn nil\n"+
		"}\n", rootName)

	// 清空数据方法
	_, _ = fmt.Fprintf(fp, "\nfunc (s *DataSet) Clear() {\n"+
		"\ts.set = nil\n"+
		"\ts.indexs = make(map[any]int, 0)\n"+
		"}\n")
	// 	sName := jsonName[:1]
	//_, _ = fmt.Fprintf(fp, "\n//go:embed %s.json\n", pkgName) // todo 指定目标路径
	// 构造方法
	_, _ = fmt.Fprintf(fp, "\nfunc _newDataSet() *DataSet {\n")
	_, _ = fmt.Fprintf(fp, "\tds := new(DataSet)\n")
	_, _ = fmt.Fprintf(fp, "\tds.set = make([]*%s, 0)\n", rootName)
	_, _ = fmt.Fprintf(fp, "\tds.indexs = make(map[any]int)\n")
	_, _ = fmt.Fprintf(fp, "\treturn ds\n")
	_, _ = fmt.Fprintf(fp, "}\n")
	// Load生产
	_, _ = fmt.Fprintf(fp, "\nfunc Load(jsonFile string) (*DataSet, error) {\n")
	_, _ = fmt.Fprintf(fp,
		"\tif _, err := os.Stat(jsonFile); err != nil {"+
			"\t\treturn nil, err\n"+
			"\t}\n"+
			"\tjsonBuf, err := os.ReadFile(jsonFile)\n"+
			"\t if err != nil {"+
			"\t\treturn nil, err\n"+
			"\t}\n",
	)
	_, _ = fmt.Fprintf(fp, "\tjsonDatas := make([]*_%sJson, 0)\n", pkgName)
	_, _ = fmt.Fprintf(fp, "\terr := json.Unmarshal(jsonBuf, &jsonDatas)\n")
	_, _ = fmt.Fprintf(fp, "\tif err != nil {\n")
	_, _ = fmt.Fprintf(fp, "\t\treturn nil, err\n")
	_, _ = fmt.Fprintf(fp, "\t}\n")
	_, _ = fmt.Fprintf(fp, "\tds := _newDataSet()\n")
	_, _ = fmt.Fprintf(fp, "\tfor index, data := range jsonDatas {\n")
	_, _ = fmt.Fprintf(fp, "\t\tobj := data._convert()\n")
	_, _ = fmt.Fprintf(fp, "\t\tds.set = append(ds.set, obj)\n")

	if rsInfo.keyIndex >= 0 {
		_, _ = fmt.Fprintf(fp, "\t\tds.indexs[obj.%s] = index\n", rsInfo.FiledList[rsInfo.keyIndex])
	} else {
		_, _ = fmt.Fprintf(fp, "\t\tds.indexs[index+1] = index\n") // 自增id
	}
	_, _ = fmt.Fprintf(fp, "\t}\n")
	_, _ = fmt.Fprintf(fp, "\treturn ds, nil\n")
	_, _ = fmt.Fprintf(fp, "}\n")
	// }
	_ = fp.Close()
	return fileName, nil
}

// 是否拥有制定结构体
func getStruct(sName string, sInfoList []*structInfo) *structInfo {
	for _, sInfo := range sInfoList {
		if sName == sInfo.Name {
			return sInfo
		}
	}
	return nil
}
