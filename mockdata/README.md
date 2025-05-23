# Mock Data for CRUD API

This directory contains sample JSON payloads for testing the CRUD API endpoints.

## API Endpoints that require JSON input:

### POST /items
Creates a new item. Requires a JSON payload with:
- `type` (string, required): The type/category of the item
- `tags` (array, optional): Tags associated with the item  
- `data` (object, required): The actual data content (must be valid JSON)

**Sample files:**
- `create_item_request.json` - Product item example
- `create_user_request.json` - User item example  
- `create_task_request.json` - Task/todo item example

### PUT /items/{id}
Updates an existing item. Same JSON structure as POST /items.

**Sample files:**
- `update_item_request.json` - Updated product with sale price

## Usage Examples

### Creating a new item:
```bash
curl -X POST http://localhost:9090/items \
  -H "Content-Type: application/json" \
  -d @mockdata/create_item_request.json
```

### Updating an item:
```bash
curl -X PUT http://localhost:9090/items/{item-id} \
  -H "Content-Type: application/json" \
  -d @mockdata/update_item_request.json
```

### Other endpoints (no JSON input needed):
- `GET /items` - List all items
- `GET /items/{id}` - Get specific item
- `DELETE /items/{id}` - Delete specific item

## Validation Rules

- `type` field cannot be empty/whitespace only
- `data` field cannot be empty and must contain valid JSON
- `tags` field is optional and can be omitted or empty array
- Request body must contain only a single JSON object (no multiple objects) 