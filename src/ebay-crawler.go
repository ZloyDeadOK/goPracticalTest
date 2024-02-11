package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

type ItemInfo struct {
	Title      string `json:"title"`
	Condition  string `json:"condition"`
	Price      string `json:"price"`
	ProductURL string `json:"product_url"`
}

const priceRegEx string = `\d+[\.,]*\d*`
const itemIDRegEx string = `itm\/([0-9]+)\?`

func main() {
	pageURL := "https://www.ebay.com/sch/garlandcomputer/m.html"

	conditionArg := flag.Int("condition", -1, "type of condition to filter. Possible values are: 3, 4 or 10.")

	flag.Parse()

	if *conditionArg != -1 {
		pageURL = fmt.Sprintf("%s?LH_ItemCondition=%d", pageURL, *conditionArg)
	}

	for {
		//Get HTML from the provided URL
		bodyHTML, err := getPageHTML(pageURL)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		//Build HTML node from HTML string
		pageHTML, err := html.Parse(strings.NewReader(bodyHTML))
		if err != nil {
			fmt.Print("Can't parse HTML\n")
			os.Exit(1)
		}

		//Get list of HTML elements with item data
		itemElementList := findItemElementsByClass(pageHTML, "li", "s-item", []*html.Node{})
		if itemElementList == nil {
			fmt.Print("Failed to get items\n")
			os.Exit(1)
		}

		if len(itemElementList) == 0 {
			fmt.Print("Failed to get items\n")
			os.Exit(1)
		}

		//Check if there are more then one page of results
		hasMorePages := false
		nextButtonNode := findFirstElementByAttr(pageHTML, "a", "class", "pagination__next icon-link")
		if nextButtonNode != nil {
			hasMorePages = true
		}

		fmt.Printf("Found %d items\n", len(itemElementList))

		os.Mkdir("data", os.ModeDir)

		//Process nodes from the current page
		wg := new(sync.WaitGroup)
		wg.Add(len(itemElementList))

		for i := 0; i < len(itemElementList); i++ {
			go processItemNode(itemElementList[i], wg)
			// if err != nil {
			// 	fmt.Printf("ERROR::Failed processing item %s\n", err)
			// }
		}

		wg.Wait()

		//If there are more pages - iterate
		if hasMorePages {
			pageURL, err = getElementAttrByName(nextButtonNode, "href")
			if err != nil {
				fmt.Printf("ERROR::Failed to get next page %s\n", err)
			}
		} else {
			break
		}
	}
}

// Function makes GET request to provided URL and returns its response in string format
func getPageHTML(url string) (string, error) {
	requestURL := url
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("ERROR::Can't create request: %s", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ERROR::Can't make http request: %s", err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("ERROR::Can't read http response body: %s", err)
	}

	return string(body), nil
}

// Function to find all indicated elements, within an HTML NODE, by Class Name
func findItemElementsByClass(node *html.Node, elementType string, className string, itemList []*html.Node) []*html.Node {
	if node.Type == html.ElementNode && node.Data == elementType {
		class := ""
		id := ""

		for _, a := range node.Attr {
			if a.Key == "class" && strings.Contains(a.Val, className) {
				class = a.Val
			} else if a.Key == "id" && a.Val != "" {
				id = a.Val
			}

			if class != "" && id != "" {
				itemList = append(itemList, node)

				break
			}
		}
	}

	for c := node.FirstChild; c != nil; c = c.NextSibling {
		itemList = findItemElementsByClass(c, elementType, className, itemList)
	}

	return itemList
}

// Function to find first element, within an HTML NODE, by Attribute
func findFirstElementByAttr(node *html.Node, elementType string, attrName string, attrValue string) *html.Node {
	nodeFound := false

	if node.Type == html.ElementNode && node.Data == elementType {
		for _, a := range node.Attr {
			if a.Key == attrName && strings.Contains(a.Val, attrValue) {
				nodeFound = true
				return node
			}
		}
	}

	for c := node.FirstChild; c != nil && !nodeFound; c = c.NextSibling {
		if c.Type == html.ElementNode {
			tempNode := findFirstElementByAttr(c, elementType, attrName, attrValue)
			if tempNode != nil {
				return tempNode
			}
		}
	}

	return nil
}

// Function to get a value of element, within an HTML NODE
func getElementNodeVal(node *html.Node) (string, error) {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			return c.Data, nil
		}
	}

	return "", fmt.Errorf("ERROR::No text node found")
}

// Function to process selected nodes (items)
func processItemNode(node *html.Node, wg *sync.WaitGroup) error {
	defer wg.Done()
	itemLink := findFirstElementByAttr(node, "a", "class", "s-item__link")
	if itemLink == nil {
		return fmt.Errorf("ERROR::Item link node not found")
	}

	href, err := getElementAttrByName(itemLink, "href")
	if err != nil {
		return fmt.Errorf("ERROR::%s", err)
	}

	re := regexp.MustCompile(itemIDRegEx)
	matches := re.FindStringSubmatch(href)
	if matches == nil {
		return fmt.Errorf("ERROR::Price value cannot be parsed\n%s", err)
	}

	if matches == nil || len(matches) < 2 {
		return fmt.Errorf("ERROR::Item ID cannot be parsed")
	}

	itemID := matches[1]

	priceNode := findFirstElementByAttr(node, "span", "class", "s-item__price")
	if priceNode == nil {
		return fmt.Errorf("ERROR::Price node not found")
	}

	price, err := getElementNodeVal(priceNode)
	if err != nil {
		return fmt.Errorf("ERROR::Price value not found\n%s", err)
	}

	re = regexp.MustCompile(priceRegEx)
	matches = re.FindStringSubmatch(price)
	if matches == nil {
		return fmt.Errorf("ERROR::Price value cannot be parsed\n%s", err)
	}

	price = matches[0]

	titleDivNode := findFirstElementByAttr(node, "div", "class", "s-item__title")
	if titleDivNode == nil {
		return fmt.Errorf("ERROR::Title DIV node not found")
	}
	titleNode := findFirstElementByAttr(titleDivNode, "span", "role", "heading")
	if titleNode == nil {
		return fmt.Errorf("ERROR::Title SPAN node not found")
	}

	title, err := getElementNodeVal(titleNode)
	if err != nil {
		return fmt.Errorf("ERROR::Title value not found\n%s", err)
	}

	condition := ""

	subtitleNode := findFirstElementByAttr(node, "div", "class", "s-item__subtitle")
	if subtitleNode == nil {
		fmt.Printf("WARNING::Condition DIV node not found %s\n", itemID)
	} else {
		conditionNode := findFirstElementByAttr(subtitleNode, "span", "class", "SECONDARY_INFO")
		if conditionNode == nil {
			return fmt.Errorf("ERROR::Condition SPAN node not found")
		}

		condition, err = getElementNodeVal(conditionNode)
		if err != nil {
			return fmt.Errorf("ERROR::Condition value not found\n%s", err)
		}
	}

	item := new(ItemInfo)
	item.Condition = condition
	item.Price = price

	item.ProductURL = href
	item.Title = title

	itemJSON, _ := json.MarshalIndent(item, "", "	")

	_ = os.WriteFile(fmt.Sprintf("data/%s.json", itemID), itemJSON, 0644)

	return nil
}

// Function to get a value of a given attribute of a node by attribute name
func getElementAttrByName(node *html.Node, attrName string) (string, error) {
	if node.Type == html.ElementNode {
		for _, a := range node.Attr {
			if a.Key == attrName {
				return a.Val, nil
			}
		}

		return "", fmt.Errorf("ERROR::Attribute %s not found", attrName)
	} else {
		return "", fmt.Errorf("ERROR::Node is not an element")
	}
}
