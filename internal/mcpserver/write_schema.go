package mcpserver

import "encoding/json"

// An explicit schema keeps RawMessage as a discoverable recursive object, and
// expresses the exclusive choice of plain body or native editor document.
var createArticleSchema = json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "required":["title"],
  "oneOf":[{"required":["body"],"not":{"required":["document"]}},{"required":["document"],"not":{"required":["body"]}}],
  "properties":{
    "title":{"type":"string","minLength":1,"description":"Nonblank title, at most 512 UTF-8 bytes."},
    "body":{"type":"string","minLength":1,"description":"Plain text, at most 256 KiB. Newlines preserved; Markdown and HTML are literal. Omit when using document."},
    "document":{"type":"object","required":["type","content"],"additionalProperties":false,"description":"Native Tiptap editor JSON, at most 256 KiB with nonblank text. Root doc contains block nodes. Text marks carry formatting. Tables must be rectangular, without merged cells or nested tables. No images, video, attachments or HTML. Omit body.","properties":{"type":{"const":"doc","type":"string"},"content":{"type":"array","minItems":1,"items":{"$ref":"#/$defs/node"}}}},
    "category_id":{"type":"string","description":"Optional existing category UUID from list_categories; can be chosen later in the editor."}
  },
  "$defs":{
    "node":{
      "type":"object","required":["type"],"additionalProperties":false,
      "properties":{
        "type":{"type":"string","enum":["paragraph","heading","text","hardBreak","bulletList","orderedList","listItem","blockquote","codeBlock","horizontalRule","table","tableRow","tableCell","tableHeader"]},
        "text":{"type":"string","minLength":1,"description":"Only for text nodes; use marks for inline formatting."},
        "content":{"type":"array","items":{"$ref":"#/$defs/node"},"description":"Child nodes. Paragraph/heading: text or hardBreak. Lists: nonempty listItem children, each starting with a paragraph. Table: nonempty tableRow children of equal cell count. Row: tableCell/tableHeader, each containing blocks."},
        "attrs":{"type":"object","additionalProperties":false,"properties":{
          "level":{"type":"integer","minimum":1,"maximum":3,"description":"heading only"},
          "start":{"type":["integer","null"],"minimum":1,"maximum":100000,"description":"orderedList only; defaults to 1"},
          "type":{"type":["string","null"],"description":"Optional editor orderedList type"},
          "language":{"type":["string","null"],"description":"codeBlock only; text children have no marks"},
          "colspan":{"type":"integer","const":1},"rowspan":{"type":"integer","const":1},
          "colwidth":{"type":["array","null"],"minItems":1,"maxItems":1,"items":{"type":"integer","minimum":0,"maximum":2000}},
          "align":{"type":["string","null"],"enum":["left","center","right",null]},
          "height":{"type":["integer","null"],"minimum":16,"maximum":2000},
          "backgroundColor":{"type":["string","null"],"pattern":"^#[0-9a-fA-F]{6}$"}
        },"description":"Only the attributes appropriate to the node type are accepted. Cell attributes apply to tableCell/tableHeader."},
        "marks":{"type":"array","maxItems":7,"items":{"$ref":"#/$defs/mark"},"description":"Text nodes only; each mark type may occur once. Inline code must be the only mark. Link needs attrs.href; textStyle uses color/backgroundColor. Other marks have no attrs."}
      }
    },
    "mark":{
      "type":"object","required":["type"],"additionalProperties":false,
      "properties":{
        "type":{"type":"string","enum":["bold","italic","underline","strike","code","link","textStyle"]},
        "attrs":{"type":"object","additionalProperties":false,"properties":{
          "href":{"type":"string","description":"link: http(s) URL without credentials, or /articles/<UUID>"},
          "title":{"type":["string","null"],"description":"Optional link tooltip, up to 4096 UTF-8 bytes"},
          "target":{"type":["string","null"]},"rel":{"type":["string","null"]},"class":{"type":["string","null"]},
          "color":{"type":["string","null"],"pattern":"^#[0-9a-fA-F]{6}$"},
          "backgroundColor":{"type":["string","null"],"pattern":"^#[0-9a-fA-F]{6}$"}
        }}
      }
    }
  }
}`)
