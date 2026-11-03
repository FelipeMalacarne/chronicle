function parsed_val2Element(parsed_val) {
	var span = document.createElement('span');
	if (parsed_val.type == 'str') {
		span.innerHTML = parsed_val.value;
	} else if (parsed_val.type == 'link') {
		var a = document.createElement('a');
		a.innerHTML = parsed_val.text;
		a.href = parsed_val.link;
		span.appendChild(a);
	} else if (parsed_val.type == 'img') {
		var img = document.createElement('img');
		img.src = parsed_val.img;
		img.style.maxWidth = '200px';
		span.appendChild(img);
	} else if (parsed_val.type == 'linkimg') {
		var img = document.createElement('img');
		img.src = parsed_val.img;
		img.style.maxWidth = '200px';
		span.appendChild(img);
		var a = document.createElement('a');
		a.innerHTML = parsed_val.text;
		a.href = parsed_val.link;
		span.appendChild(a);
	}
	return span;
}

function handleRefs(refs) {
	var out = document.createElement('div');
	var header = document.createElement('h3');
	header.innerHTML = 'References';
	if (refs['cs2'].length + refs['structs'].length + refs['enums'].length > 0) {
		out.appendChild(header);
	}
	var types = ['cs2', 'structs', 'enums'];
	types.forEach(function(type) {
		if (refs[type].length > 0) {
			var typeHeader = document.createElement('h4');
			typeHeader.innerHTML = type;
			out.appendChild(typeHeader);
			var list = document.createElement('ul');
			refs[type].forEach(function(ref) {
				var li = document.createElement('li');
				li.appendChild(parsed_val2Element(ref));
				list.appendChild(li)
			})
			out.appendChild(list);
		}
	})
	return out;

}
function handleEnums(id, data) {
	const main = document.getElementById("main");
	var header = document.createElement('h2');
	var headerText = 'ENUM #' + id;
	if (data.name) {
		headerText += ': ' + data.name
	}
	header.innerHTML = headerText;

	main.appendChild(header);
	var table = document.createElement('table');
	table.border = '1';
	var thead = document.createElement('tr');
	var th1 = document.createElement('th');
	th1.innerHTML = 'raw_key';
	var th2 = document.createElement('th');
	th2.innerHTML = 'raw_val';
	var th3 = document.createElement('th');
	th3.innerHTML = 'key_type';
	var th4 = document.createElement('th');
	th4.innerHTML = 'val_type';
	var th5 = document.createElement('th');
	th5.innerHTML = 'parsed_key';
	var th6 = document.createElement('th');
	th6.innerHTML = 'parsed_val';
	thead.appendChild(th1);
	thead.appendChild(th2);
	thead.appendChild(th3);
	thead.appendChild(th4);
	thead.appendChild(th5);
	thead.appendChild(th6);
	table.appendChild(thead);
	data.rows.forEach(function(row) { 
		var tr = document.createElement('tr');
		var td1 = document.createElement('td');
		td1.innerHTML = row.raw_key;
		var td2 = document.createElement('td');
		td2.innerHTML = row.raw_val;
		var td3 = document.createElement('td');
		td3.innerHTML = row.key_type;
		var td4 = document.createElement('td');
		td4.innerHTML = row.val_type;
		var td5 = document.createElement('td');
		td5.appendChild(parsed_val2Element(row.parsed_key));
		var td6 = document.createElement('td');
		td6.appendChild(parsed_val2Element(row.parsed_val));
		tr.appendChild(td1);
		tr.appendChild(td2);
		tr.appendChild(td3);
		tr.appendChild(td4);
		tr.appendChild(td5);
		tr.appendChild(td6);
		table.appendChild(tr);
	})

	main.appendChild(table);
	main.appendChild(handleRefs(data.references))
}

function handleParams(id, data) {
	const main = document.getElementById("main");
	var header = document.createElement('h2');
	var headerText = 'PARAM #' + id;
	if (data.name) {
		headerText += ': ' + data.name
	}
	header.innerHTML = headerText;
	main.appendChild(header);
	var typeP = document.createElement('p')
	typeP.innerHTML = 'Type: ' + data.type;
	main.appendChild(typeP);
	var defP = document.createElement('p')
	defP.innerHTML = 'Default: ';
	defP.appendChild(parsed_val2Element(data.default))
	main.appendChild(defP);
	var table = document.createElement('table');
	table.border = '1';
	var thead = document.createElement('tr');
	var th1 = document.createElement('th');
	th1.innerHTML = 'source';
	var th2 = document.createElement('th');
	th2.innerHTML = 'raw_val';
	var th3 = document.createElement('th');
	th3.innerHTML = 'parsed_val';
	thead.appendChild(th1);
	thead.appendChild(th2);
	thead.appendChild(th3);
	table.appendChild(thead);

	data.rows.forEach(function(row) { 
		var tr = document.createElement('tr');
		var td1 = document.createElement('td');
		td1.appendChild(parsed_val2Element(row.source))
		var td2 = document.createElement('td');
		td2.innerHTML = row.raw_val;
		var td3 = document.createElement('td');
		td3.appendChild(parsed_val2Element(row.parsed_val))
		tr.appendChild(td1);
		tr.appendChild(td2);
		tr.appendChild(td3);
		table.appendChild(tr);
	})

	main.appendChild(table);
	main.appendChild(handleRefs(data.references))

}

function handleStructs(id, data) {
	const main = document.getElementById("main");
	var header = document.createElement('h2');
	var headerText = 'STRUCT #' + id;
	if (data.name) {
		headerText += ': ' + data.name
	}
	header.innerHTML = headerText;

	main.appendChild(header);

	var table = document.createElement('table');
	table.border = '1';
	var thead = document.createElement('tr');
	var th1 = document.createElement('th');
	th1.innerHTML = 'raw_key';
	var th2 = document.createElement('th');
	th2.innerHTML = 'raw_val';
	var th3 = document.createElement('th');
	th3.innerHTML = 'type';
	var th4 = document.createElement('th');
	th4.innerHTML = 'key_name';
	var th5 = document.createElement('th');
	th5.innerHTML = 'parsed_key';
	thead.appendChild(th1);
	thead.appendChild(th2);
	thead.appendChild(th3);
	thead.appendChild(th4);
	thead.appendChild(th5);
	table.appendChild(thead);
	data.rows.forEach(function(row) { 
		var tr = document.createElement('tr');
		var td1 = document.createElement('td');
		var a = document.createElement('a');
		a.href = `index.html?type=params&id=${row.raw_key}`;
		a.innerHTML = row.raw_key;
		td1.appendChild(a)
		var td2 = document.createElement('td');
		td2.innerHTML = row.raw_val;
		var td3 = document.createElement('td');
		td3.innerHTML = row.type;
		var td4 = document.createElement('td');
		td4.innerHTML = row.key_name;
		var td5 = document.createElement('td');
		td5.appendChild(parsed_val2Element(row.parsed_val));
		tr.appendChild(td1);
		tr.appendChild(td2);
		tr.appendChild(td3);
		tr.appendChild(td4);
		tr.appendChild(td5);
		table.appendChild(tr);
	})

	main.appendChild(table);
	main.appendChild(handleRefs(data.references))
}

function handleIndex() {
	const main = document.getElementById("main");
	var ul = document.createElement('ul')
	var types = ['params', 'structs', 'enums']
	types.forEach(function(type) {
		var li = document.createElement('li')
		var a = document.createElement('a')
		a.innerHTML = type
		a.href = `index.html?type=${type}`
		li.appendChild(a)
		ul.appendChild(li)
	})
	main.appendChild(ul)
}

function enumsAll() {
	const main = document.getElementById("main");
	var table = document.createElement('table')
	table.border = 1;
	var thead = document.createElement('tr')
	var headers = ['id', 'name', 'size', 'keyType', 'valType', 'refs'];
	headers.forEach(header => {
		var th = document.createElement('th')
		th.innerHTML = header
		thead.appendChild(th)
	})
	table.appendChild(thead)
	fetch('enums/all.json')
		.then(response => response.json())
		.then(data => {
			data.forEach(row => {
				var tr = document.createElement('tr')
				if (row.size == 0) {
					tr.style.backgroundColor = 'red';
				}
				if (row.name) {
					tr.style.backgroundColor = 'green';
				}
				var elts = [`<a href="index.html?type=enums&id=${row.id}">${row.id}</a>`, row.name, row.size, row.keyType, row.valType, row.refs];
				elts.forEach(elt => {
					var td = document.createElement('td')
					td.innerHTML = elt
					tr.appendChild(td)
				});
				table.appendChild(tr);
			})
		})
	main.appendChild(table)
}

function paramsAll() {
	const main = document.getElementById("main");
	var table = document.createElement('table')
	table.border = 1;
	var thead = document.createElement('tr')
	var headers = ['id', 'type', 'name', 'size', 'refs', 'example'];
	headers.forEach(header => {
		var th = document.createElement('th')
		th.innerHTML = header
		thead.appendChild(th)
	})
	table.appendChild(thead)
	fetch('params/all.json')
		.then(response => response.json())
		.then(data => {
			data.forEach(row => {
				var tr = document.createElement('tr')
				if (row.size == 0) {
					tr.style.backgroundColor = 'red';
				}
				if (row.name) {
					tr.style.backgroundColor = 'green';
				}
				var elts = [`<a href="index.html?type=params&id=${row.id}">${row.id}</a>`, row.type, row.name, row.size, row.refs, parsed_val2Element(row.example).outerHTML];
				elts.forEach(elt => {
					var td = document.createElement('td')
					td.innerHTML = elt
					tr.appendChild(td)
				});
				table.appendChild(tr);
			})
		})
	main.appendChild(table)
}

function structsAll() {
	const main = document.getElementById("main");
	var table = document.createElement('table')
	table.border = 1;
	var thead = document.createElement('tr')
	var headers = ['id', 'name', 'size', 'refs'];
	headers.forEach(header => {
		var th = document.createElement('th')
		th.innerHTML = header
		thead.appendChild(th)
	})
	table.appendChild(thead)
	fetch('structs/all.json')
		.then(response => response.json())
		.then(data => {
			data.forEach(row => {
				var tr = document.createElement('tr')
				if (row.size == 0) {
					tr.style.backgroundColor = 'red';
				}
				if (row.name) {
					tr.style.backgroundColor = 'green';
				}
				var elts = [`<a href="index.html?type=structs&id=${row.id}">${row.id}</a>`, row.name, row.size, row.refs];
				elts.forEach(elt => {
					var td = document.createElement('td')
					td.innerHTML = elt
					tr.appendChild(td)
				});
				table.appendChild(tr);
			})
		})
	main.appendChild(table)
}

function main() {
	const queryString = window.location.search;

	const urlParams = new URLSearchParams(queryString);
	const id = urlParams.get('id');
	const pageType = urlParams.get('type');
	if (id === null) {
	  if (pageType === 'structs') {
	  	return structsAll()
	  } else if (pageType === 'enums') {
	  	return enumsAll()
	  } else if (pageType === 'params') {
	  	return paramsAll()
	  } else {
	  	return handleIndex()
	  }
	}

	const url = `${pageType}/${id}.json`;
	var handleFuncs = {'structs': handleStructs, 'enums': handleEnums, 'params': handleParams}
	var handle = handleFuncs[pageType];
	fetch(url)
		.then(response => response.json())
		.then(data => handle(id, data))
		.catch((error) => {
			document.getElementById("main").innerHTML = "Could not find any data for this entity."
	  		console.log(error)
		});
}

window.onload = main
