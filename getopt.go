package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"gopkg.in/yaml.v2"
)

const (
	yml = "getoptions.yaml"
	fn  = "getopt_long.c"
)

type Options []struct {
	Option struct {
		Name         string `yaml:"name"`
		Abbreviation string `yaml:"abbreviation"`
		HasArg       struct {
			Type             string `yaml:"type"`
			OptionalArgument struct {
				Type   string   `yaml:"type"`
				Values []string `yaml:"values"`
			} `yaml:"optional_argument"`
		} `yaml:"has_arg"`
	} `yaml:"option"`
}

type TemplateOption struct {
	OptionDef  string
	OptionTest string
}

type TemplateData struct {
	Option       []TemplateOption
	OptionString string
}

var getopt_long_c_prg = template.Must(template.ParseFiles("getopt_long.c.gotemplate"))

func main() {
	data, err := ioutil.ReadFile(yml)
	if err != nil {
		log.Fatalf("error: could not read file: %v", err)
	}

	var opts Options
	err = yaml.Unmarshal(data, &opts)
	if err != nil {
		log.Fatalf("cannot unmarshal data: %v", err)
	}

	sort.Slice(opts, func(i, j int) bool {
		return opts[i].Option.Abbreviation < opts[j].Option.Abbreviation
	})

	var templ_opts []TemplateOption
	var optstring string
	for _, opt := range opts {
		optdef := fmt.Sprintf("{% 15q, % 20v, 0, '%c'}",
			opt.Option.Name,
			opt.Option.HasArg.Type,
			opt.Option.Abbreviation[0])

		var opttest string
		switch opt.Option.HasArg.Type {
		case "no_argument":
			optstring += fmt.Sprintf("%c", opt.Option.Abbreviation[0])
			opttest = fmt.Sprintf(`
            case '%c':
                puts ("option -%c was given\n");
                break;
            `, opt.Option.Abbreviation[0], opt.Option.Abbreviation[0])
		case "required_argument":
			optstring += fmt.Sprintf("%c:", opt.Option.Abbreviation[0])
			opttest = fmt.Sprintf(`
            case '%c':
                printf ("option -%c was given with value '%%s'\n", optarg);
                break;
            `, opt.Option.Abbreviation[0], opt.Option.Abbreviation[0])
		case "optional_argument":
			optstring += fmt.Sprintf("%c::", opt.Option.Abbreviation[0])
			opttest = fmt.Sprintf(`
            case '%c':
                if (optarg)
                    printf ("option -%c was given with value '%%s'\n", optarg);
                else
                    printf ("option -%c was given witout value\n");
                break;
            `, opt.Option.Abbreviation[0], opt.Option.Abbreviation[0],
				opt.Option.Abbreviation[0])
		default:
		}

		templ_opts = append(templ_opts, TemplateOption{
			OptionDef:  optdef,
			OptionTest: opttest,
		})
	}
	template_data := TemplateData{
		Option:       templ_opts,
		OptionString: optstring,
	}

	f, err := os.Create(fn)
	if err != nil {
		log.Fatalf("error: could not create file: %v", err)
	}

	if err := getopt_long_c_prg.Execute(f, template_data); err != nil {
		log.Fatalf("error: could execute template: %v", err)
	}
	f.Close()
	// Postprocess with clang-format
	//applyClangFormat(fn)

	fmt.Println("Now, we test this code by using:")
	testOutputWithCompiler("gcc", fn)
	testOutputWithCompiler("clang", fn)
}

func applyClangFormat(fn string) {
	clangformat := exec.Command("clang-format", fn)
	stdout, err := clangformat.StdoutPipe()
	if err != nil {
		fmt.Println("Could not redirect stdout of clang-format", err)
		return
	}
	reader := bufio.NewReader(stdout)
	if err = clangformat.Start(); err != nil {
		fmt.Println("Could not start clang-format", err)
		return
	}

	tmpfile, err := ioutil.TempFile("", "clangformat")
	if err != nil {
		fmt.Println("could create temporary file for applying clang-format", err)
		return
	}
	//defer os.Remove(tmpfile.Name())

	for {
		buf := make([]byte, 4096)
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println("Could not read from stdout of clang-format", err)
			return
		}
		n, err = tmpfile.Write(buf[:n])
		if err != nil {
			fmt.Println("could not write to temporary file", err)
			return
		}
	}
	tmpfile.Close()

	if clangformat.Wait(); err != nil {
		fmt.Println("Wait() failed for clang-format", err)
		return
	}

	// Copy tmpfile content to original file
	src, err := os.Open(tmpfile.Name())
	if err != nil {
		fmt.Println("could not open temporary file", err)
		return
	}
	defer src.Close()

	dst, err := os.Create(fn)
	if err != nil {
		fmt.Println("could not open target file", fn, err)
		return
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		fmt.Println("could not copy temporary file to", fn, err)
		return
	}
}

func testOutputWithCompiler(compiler string, inputfile string) {
	args := []string{
		"-std=c11",
		"-Wall",
		"-Werror",
		"-pthread",
		"-o",
		strings.TrimSuffix(inputfile, filepath.Ext(inputfile)) + "-" + compiler,
		inputfile,
	}
	s := fmt.Sprint(compiler)
	for _, arg := range args {
		s += fmt.Sprintf(" %s", arg)
	}
	s += fmt.Sprint(": ")

	cmd := exec.Command(compiler, args...)
	err := cmd.Run()
	fmt.Printf("% 74s", s)
	if err != nil {
		color.Red(" % 10s ", "[failed]")
		fmt.Println(err)
	} else {
		color.Green(" % 5s ", "[OK]")
	}
}
