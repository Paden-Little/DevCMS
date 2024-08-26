from flask import Flask, request, jsonify
import uuid
import m2plib
import os
import json
app = Flask(__name__)


def render(rawMD):
    return m2plib.markdown(rawMD, extras=['code-friendly', 'latex', 'fenced-code-blocks'])


def handle_get(request):
    if 'name' in request.args:  # curl -X GET "http://127.0.0.1:5000/posts?name={name}" Get specific Post
        if 'name' in request.args:  # curl -X GET "http://127.0.0.1:5000/posts?name={name}" Get specific Post
            filename = request.args.get('name')
            file_path = os.path.join('posts/', f"{filename}.md")

            if os.path.exists(file_path):
                with open(file_path, 'r', encoding='utf-8') as file:
                    lines = file.readlines()

                # Check if the first line is metadata
                first_line = lines[0].strip()
                if first_line.startswith('{') and first_line.endswith('}'):
                    metadata = json.loads(first_line)
                    content = ''.join(lines[1:])  # Content without metadata
                else:
                    metadata = {
                        'description': None,
                        'title': None,
                        'datepublished': None,
                        'tags': None,
                        'filename': None
                    }
                    content = ''.join(lines)  # Content with no metadata found

                # Render the content without the metadata
                rendered_content = render(content)

                return jsonify({
                    'metadata': metadata,
                    'content': rendered_content
                })
            else:
                return jsonify({'error': 'File not found'}), 404
    else:  # curl -X GET "http://127.0.0.1:5000/posts Get all posts
        posts_directory = 'posts/'
        metadata_list = []

        for filename in os.listdir(posts_directory):
            if filename.endswith('.md'):
                file_path = os.path.join(posts_directory, filename)

                with open(file_path, 'r', encoding='utf-8') as file:
                    first_line = file.readline().strip()

                    if first_line.startswith('{') and first_line.endswith('}'):
                        metadata = json.loads(first_line)
                        metadata['filename'] = os.path.splitext(filename)[
                            0]  # Add the filename (without extension) to the metadata
                        metadata_list.append(metadata)
                    else:
                        metadata_list.append({
                            'filename': os.path.splitext(filename)[0],
                            'description': None,
                            'title': None,
                            'datepublished': None,
                            'tags': None
                        })

        return jsonify(metadata_list)


def handle_post(request):
    f = request.files['file']
    new_filename = f.filename
    save_path = os.path.join('posts/', new_filename)

    # Check if the file already exists
    if os.path.exists(save_path):
        return jsonify({'error': 'File with that name already exists'}), 409

    # Save the file
    f.save(save_path)
    return jsonify({'message': 'File saved', 'filename': new_filename})


def handle_patch(request):
    filename = request.args.get('filePath')

    if not filename:
        return jsonify({'error': 'Filename query parameter is required'}), 400

    file_path = os.path.join('posts/', f"{filename}.md")

    if not os.path.exists(file_path):
        return jsonify({'error': 'File not found'}), 404

    metadata = request.get_json()

    metadata.update({'filename': filename})

    print(metadata)

    required_fields = ['description', 'title', 'datepublished', 'tags']
    if not all(field in metadata for field in required_fields):
        return jsonify({'error': f"Missing required fields. Required fields are: {required_fields}"}), 400

    # Convert metadata to a compact JSON string without whitespace
    metadata_str = json.dumps(metadata, separators=(',', ':'))

    try:
        with open(file_path, 'r', encoding='utf-8') as file:
            lines = file.readlines()

        if lines:
            first_line = lines[0].strip()
            try:
                # Attempt to parse the first line as JSON
                json.loads(first_line)
                # If successful, replace the first line with new metadata
                lines[0] = metadata_str + '\n'
            except json.JSONDecodeError:
                # If parsing fails, prepend the new metadata
                lines.insert(0, metadata_str + '\n')
        else:
            # If the file is empty, simply add the metadata
            lines = [metadata_str + '\n']

        # Write the modified lines back to the file
        with open(file_path, 'w', encoding='utf-8') as file:
            file.writelines(lines)

        return jsonify({'message': f'Metadata updated for {filename}.md successfully'}), 200

    except Exception as e:
        return jsonify({'error': f"An error occurred: {str(e)}"}), 500



def handle_put(request):
    filename = request.args.get('filename')

    if filename:
        file_path = os.path.join('posts/', f"{filename}.md")

        # Check if the file exists
        if os.path.exists(file_path):
            # Overwrite the existing file with new content
            f = request.files['data']
            f.save(file_path)
            return jsonify({'message': f'File {filename}.md updated successfully'})
        else:
            return jsonify({'error': 'File not found'}), 404
    else:
        return jsonify({'error': 'Filename query parameter is required'}), 400


def handle_delete(request):
    filename = request.args.get('filename')

    if filename:
        file_path = os.path.join('posts/', f"{filename}.md")

        # Check if the file exists
        if os.path.exists(file_path):
            os.remove(file_path)  # Delete the file
            return jsonify({'message': f'File {filename}.md deleted successfully'})
        else:
            return jsonify({'error': 'File not found'}), 404
    else:
        return jsonify({'error': 'Filename query parameter is required'}), 400


@app.route('/posts', methods=['GET', 'POST', 'PATCH', 'PUT', 'DELETE'])
def item_handler():
    if request.method == 'GET':
        return handle_get(request)
    elif request.method == 'POST':  # curl -X POST -F "data=@foo.md" "http://127.0.0.1:5000/posts"
        return handle_post(request)
    elif request.method == 'PATCH': # curl -X PATCH -
        return handle_patch(request)
    elif request.method == 'PUT':  # curl -X PUT -F "data=@foo.md" "http://127.0.0.1:5000/posts?filename=foo"
        return handle_put(request)
    elif request.method == 'DELETE':  # curl -X DELETE "http://127.0.0.1:5000/posts?filename=foo"
        return handle_delete(request)


if __name__ == '__main__':
    app.run()