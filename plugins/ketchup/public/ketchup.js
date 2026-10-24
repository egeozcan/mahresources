/*! ketchup d93fb38525f79c3efdcdb6ccc0a1dbbe3474aa12 (main) from https://github.com/egeozcan/ketchup.git, built by scripts/update-ketchup.sh with node v25.3.0 */
/**
 * @license
 * Copyright 2019 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const bs=globalThis,Ui=bs.ShadowRoot&&(bs.ShadyCSS===void 0||bs.ShadyCSS.nativeShadow)&&"adoptedStyleSheets"in Document.prototype&&"replace"in CSSStyleSheet.prototype,Vi=Symbol(),mr=new WeakMap;let Wn=class{constructor(t,s,i){if(this._$cssResult$=!0,i!==Vi)throw Error("CSSResult is not constructable. Use `unsafeCSS` or `css` instead.");this.cssText=t,this.t=s}get styleSheet(){let t=this.o;const s=this.t;if(Ui&&t===void 0){const i=s!==void 0&&s.length===1;i&&(t=mr.get(s)),t===void 0&&((this.o=t=new CSSStyleSheet).replaceSync(this.cssText),i&&mr.set(s,t))}return t}toString(){return this.cssText}};const Za=e=>new Wn(typeof e=="string"?e:e+"",void 0,Vi),le=(e,...t)=>{const s=e.length===1?e[0]:t.reduce((i,r,n)=>i+(a=>{if(a._$cssResult$===!0)return a.cssText;if(typeof a=="number")return a;throw Error("Value passed to 'css' function must be a 'css' function result: "+a+". Use 'unsafeCSS' to pass non-literal values, but take care to ensure page security.")})(r)+e[n+1],e[0]);return new Wn(s,e,Vi)},Qa=(e,t)=>{if(Ui)e.adoptedStyleSheets=t.map(s=>s instanceof CSSStyleSheet?s:s.styleSheet);else for(const s of t){const i=document.createElement("style"),r=bs.litNonce;r!==void 0&&i.setAttribute("nonce",r),i.textContent=s.cssText,e.appendChild(i)}},gr=Ui?e=>e:e=>e instanceof CSSStyleSheet?(t=>{let s="";for(const i of t.cssRules)s+=i.cssText;return Za(s)})(e):e;/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const{is:Ja,defineProperty:to,getOwnPropertyDescriptor:eo,getOwnPropertyNames:so,getOwnPropertySymbols:io,getPrototypeOf:ro}=Object,Fs=globalThis,yr=Fs.trustedTypes,no=yr?yr.emptyScript:"",ao=Fs.reactiveElementPolyfillSupport,Ze=(e,t)=>e,Ps={toAttribute(e,t){switch(t){case Boolean:e=e?no:null;break;case Object:case Array:e=e==null?e:JSON.stringify(e)}return e},fromAttribute(e,t){let s=e;switch(t){case Boolean:s=e!==null;break;case Number:s=e===null?null:Number(e);break;case Object:case Array:try{s=JSON.parse(e)}catch{s=null}}return s}},Gi=(e,t)=>!Ja(e,t),vr={attribute:!0,type:String,converter:Ps,reflect:!1,useDefault:!1,hasChanged:Gi};Symbol.metadata??=Symbol("metadata"),Fs.litPropertyMetadata??=new WeakMap;let De=class extends HTMLElement{static addInitializer(t){this._$Ei(),(this.l??=[]).push(t)}static get observedAttributes(){return this.finalize(),this._$Eh&&[...this._$Eh.keys()]}static createProperty(t,s=vr){if(s.state&&(s.attribute=!1),this._$Ei(),this.prototype.hasOwnProperty(t)&&((s=Object.create(s)).wrapped=!0),this.elementProperties.set(t,s),!s.noAccessor){const i=Symbol(),r=this.getPropertyDescriptor(t,i,s);r!==void 0&&to(this.prototype,t,r)}}static getPropertyDescriptor(t,s,i){const{get:r,set:n}=eo(this.prototype,t)??{get(){return this[s]},set(a){this[s]=a}};return{get:r,set(a){const o=r?.call(this);n?.call(this,a),this.requestUpdate(t,o,i)},configurable:!0,enumerable:!0}}static getPropertyOptions(t){return this.elementProperties.get(t)??vr}static _$Ei(){if(this.hasOwnProperty(Ze("elementProperties")))return;const t=ro(this);t.finalize(),t.l!==void 0&&(this.l=[...t.l]),this.elementProperties=new Map(t.elementProperties)}static finalize(){if(this.hasOwnProperty(Ze("finalized")))return;if(this.finalized=!0,this._$Ei(),this.hasOwnProperty(Ze("properties"))){const s=this.properties,i=[...so(s),...io(s)];for(const r of i)this.createProperty(r,s[r])}const t=this[Symbol.metadata];if(t!==null){const s=litPropertyMetadata.get(t);if(s!==void 0)for(const[i,r]of s)this.elementProperties.set(i,r)}this._$Eh=new Map;for(const[s,i]of this.elementProperties){const r=this._$Eu(s,i);r!==void 0&&this._$Eh.set(r,s)}this.elementStyles=this.finalizeStyles(this.styles)}static finalizeStyles(t){const s=[];if(Array.isArray(t)){const i=new Set(t.flat(1/0).reverse());for(const r of i)s.unshift(gr(r))}else t!==void 0&&s.push(gr(t));return s}static _$Eu(t,s){const i=s.attribute;return i===!1?void 0:typeof i=="string"?i:typeof t=="string"?t.toLowerCase():void 0}constructor(){super(),this._$Ep=void 0,this.isUpdatePending=!1,this.hasUpdated=!1,this._$Em=null,this._$Ev()}_$Ev(){this._$ES=new Promise(t=>this.enableUpdating=t),this._$AL=new Map,this._$E_(),this.requestUpdate(),this.constructor.l?.forEach(t=>t(this))}addController(t){(this._$EO??=new Set).add(t),this.renderRoot!==void 0&&this.isConnected&&t.hostConnected?.()}removeController(t){this._$EO?.delete(t)}_$E_(){const t=new Map,s=this.constructor.elementProperties;for(const i of s.keys())this.hasOwnProperty(i)&&(t.set(i,this[i]),delete this[i]);t.size>0&&(this._$Ep=t)}createRenderRoot(){const t=this.shadowRoot??this.attachShadow(this.constructor.shadowRootOptions);return Qa(t,this.constructor.elementStyles),t}connectedCallback(){this.renderRoot??=this.createRenderRoot(),this.enableUpdating(!0),this._$EO?.forEach(t=>t.hostConnected?.())}enableUpdating(t){}disconnectedCallback(){this._$EO?.forEach(t=>t.hostDisconnected?.())}attributeChangedCallback(t,s,i){this._$AK(t,i)}_$ET(t,s){const i=this.constructor.elementProperties.get(t),r=this.constructor._$Eu(t,i);if(r!==void 0&&i.reflect===!0){const n=(i.converter?.toAttribute!==void 0?i.converter:Ps).toAttribute(s,i.type);this._$Em=t,n==null?this.removeAttribute(r):this.setAttribute(r,n),this._$Em=null}}_$AK(t,s){const i=this.constructor,r=i._$Eh.get(t);if(r!==void 0&&this._$Em!==r){const n=i.getPropertyOptions(r),a=typeof n.converter=="function"?{fromAttribute:n.converter}:n.converter?.fromAttribute!==void 0?n.converter:Ps;this._$Em=r;const o=a.fromAttribute(s,n.type);this[r]=o??this._$Ej?.get(r)??o,this._$Em=null}}requestUpdate(t,s,i,r=!1,n){if(t!==void 0){const a=this.constructor;if(r===!1&&(n=this[t]),i??=a.getPropertyOptions(t),!((i.hasChanged??Gi)(n,s)||i.useDefault&&i.reflect&&n===this._$Ej?.get(t)&&!this.hasAttribute(a._$Eu(t,i))))return;this.C(t,s,i)}this.isUpdatePending===!1&&(this._$ES=this._$EP())}C(t,s,{useDefault:i,reflect:r,wrapped:n},a){i&&!(this._$Ej??=new Map).has(t)&&(this._$Ej.set(t,a??s??this[t]),n!==!0||a!==void 0)||(this._$AL.has(t)||(this.hasUpdated||i||(s=void 0),this._$AL.set(t,s)),r===!0&&this._$Em!==t&&(this._$Eq??=new Set).add(t))}async _$EP(){this.isUpdatePending=!0;try{await this._$ES}catch(s){Promise.reject(s)}const t=this.scheduleUpdate();return t!=null&&await t,!this.isUpdatePending}scheduleUpdate(){return this.performUpdate()}performUpdate(){if(!this.isUpdatePending)return;if(!this.hasUpdated){if(this.renderRoot??=this.createRenderRoot(),this._$Ep){for(const[r,n]of this._$Ep)this[r]=n;this._$Ep=void 0}const i=this.constructor.elementProperties;if(i.size>0)for(const[r,n]of i){const{wrapped:a}=n,o=this[r];a!==!0||this._$AL.has(r)||o===void 0||this.C(r,void 0,n,o)}}let t=!1;const s=this._$AL;try{t=this.shouldUpdate(s),t?(this.willUpdate(s),this._$EO?.forEach(i=>i.hostUpdate?.()),this.update(s)):this._$EM()}catch(i){throw t=!1,this._$EM(),i}t&&this._$AE(s)}willUpdate(t){}_$AE(t){this._$EO?.forEach(s=>s.hostUpdated?.()),this.hasUpdated||(this.hasUpdated=!0,this.firstUpdated(t)),this.updated(t)}_$EM(){this._$AL=new Map,this.isUpdatePending=!1}get updateComplete(){return this.getUpdateComplete()}getUpdateComplete(){return this._$ES}shouldUpdate(t){return!0}update(t){this._$Eq&&=this._$Eq.forEach(s=>this._$ET(s,this[s])),this._$EM()}updated(t){}firstUpdated(t){}};De.elementStyles=[],De.shadowRootOptions={mode:"open"},De[Ze("elementProperties")]=new Map,De[Ze("finalized")]=new Map,ao?.({ReactiveElement:De}),(Fs.reactiveElementVersions??=[]).push("2.1.2");/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const qi=globalThis,br=e=>e,Ts=qi.trustedTypes,wr=Ts?Ts.createPolicy("lit-html",{createHTML:e=>e}):void 0,Yn="$lit$",he=`lit$${Math.random().toFixed(9).slice(2)}$`,Un="?"+he,oo=`<${Un}>`,Se=document,es=()=>Se.createComment(""),ss=e=>e===null||typeof e!="object"&&typeof e!="function",Ki=Array.isArray,ho=e=>Ki(e)||typeof e?.[Symbol.iterator]=="function",Zs=`[ 	
\f\r]`,Xe=/<(?:(!--|\/[^a-zA-Z])|(\/?[a-zA-Z][^>\s]*)|(\/?$))/g,xr=/-->/g,kr=/>/g,ue=RegExp(`>|${Zs}(?:([^\\s"'>=/]+)(${Zs}*=${Zs}*(?:[^ 	
\f\r"'\`<>=]|("|')|))|$)`,"g"),Mr=/'/g,Cr=/"/g,Vn=/^(?:script|style|textarea|title)$/i,Gn=e=>(t,...s)=>({_$litType$:e,strings:t,values:s}),v=Gn(1),W=Gn(2),Pe=Symbol.for("lit-noChange"),A=Symbol.for("lit-nothing"),Sr=new WeakMap,we=Se.createTreeWalker(Se,129);function qn(e,t){if(!Ki(e)||!e.hasOwnProperty("raw"))throw Error("invalid template strings array");return wr!==void 0?wr.createHTML(t):t}const co=(e,t)=>{const s=e.length-1,i=[];let r,n=t===2?"<svg>":t===3?"<math>":"",a=Xe;for(let o=0;o<s;o++){const h=e[o];let c,l,d=-1,u=0;for(;u<h.length&&(a.lastIndex=u,l=a.exec(h),l!==null);)u=a.lastIndex,a===Xe?l[1]==="!--"?a=xr:l[1]!==void 0?a=kr:l[2]!==void 0?(Vn.test(l[2])&&(r=RegExp("</"+l[2],"g")),a=ue):l[3]!==void 0&&(a=ue):a===ue?l[0]===">"?(a=r??Xe,d=-1):l[1]===void 0?d=-2:(d=a.lastIndex-l[2].length,c=l[1],a=l[3]===void 0?ue:l[3]==='"'?Cr:Mr):a===Cr||a===Mr?a=ue:a===xr||a===kr?a=Xe:(a=ue,r=void 0);const _=a===ue&&e[o+1].startsWith("/>")?" ":"";n+=a===Xe?h+oo:d>=0?(i.push(c),h.slice(0,d)+Yn+h.slice(d)+he+_):h+he+(d===-2?o:_)}return[qn(e,n+(e[s]||"<?>")+(t===2?"</svg>":t===3?"</math>":"")),i]};class is{constructor({strings:t,_$litType$:s},i){let r;this.parts=[];let n=0,a=0;const o=t.length-1,h=this.parts,[c,l]=co(t,s);if(this.el=is.createElement(c,i),we.currentNode=this.el.content,s===2||s===3){const d=this.el.content.firstChild;d.replaceWith(...d.childNodes)}for(;(r=we.nextNode())!==null&&h.length<o;){if(r.nodeType===1){if(r.hasAttributes())for(const d of r.getAttributeNames())if(d.endsWith(Yn)){const u=l[a++],_=r.getAttribute(d).split(he),f=/([.?@])?(.*)/.exec(u);h.push({type:1,index:n,name:f[2],strings:_,ctor:f[1]==="."?uo:f[1]==="?"?po:f[1]==="@"?fo:Ns}),r.removeAttribute(d)}else d.startsWith(he)&&(h.push({type:6,index:n}),r.removeAttribute(d));if(Vn.test(r.tagName)){const d=r.textContent.split(he),u=d.length-1;if(u>0){r.textContent=Ts?Ts.emptyScript:"";for(let _=0;_<u;_++)r.append(d[_],es()),we.nextNode(),h.push({type:2,index:++n});r.append(d[u],es())}}}else if(r.nodeType===8)if(r.data===Un)h.push({type:2,index:n});else{let d=-1;for(;(d=r.data.indexOf(he,d+1))!==-1;)h.push({type:7,index:n}),d+=he.length-1}n++}}static createElement(t,s){const i=Se.createElement("template");return i.innerHTML=t,i}}function ze(e,t,s=e,i){if(t===Pe)return t;let r=i!==void 0?s._$Co?.[i]:s._$Cl;const n=ss(t)?void 0:t._$litDirective$;return r?.constructor!==n&&(r?._$AO?.(!1),n===void 0?r=void 0:(r=new n(e),r._$AT(e,s,i)),i!==void 0?(s._$Co??=[])[i]=r:s._$Cl=r),r!==void 0&&(t=ze(e,r._$AS(e,t.values),r,i)),t}class lo{constructor(t,s){this._$AV=[],this._$AN=void 0,this._$AD=t,this._$AM=s}get parentNode(){return this._$AM.parentNode}get _$AU(){return this._$AM._$AU}u(t){const{el:{content:s},parts:i}=this._$AD,r=(t?.creationScope??Se).importNode(s,!0);we.currentNode=r;let n=we.nextNode(),a=0,o=0,h=i[0];for(;h!==void 0;){if(a===h.index){let c;h.type===2?c=new He(n,n.nextSibling,this,t):h.type===1?c=new h.ctor(n,h.name,h.strings,this,t):h.type===6&&(c=new _o(n,this,t)),this._$AV.push(c),h=i[++o]}a!==h?.index&&(n=we.nextNode(),a++)}return we.currentNode=Se,r}p(t){let s=0;for(const i of this._$AV)i!==void 0&&(i.strings!==void 0?(i._$AI(t,i,s),s+=i.strings.length-2):i._$AI(t[s])),s++}}class He{get _$AU(){return this._$AM?._$AU??this._$Cv}constructor(t,s,i,r){this.type=2,this._$AH=A,this._$AN=void 0,this._$AA=t,this._$AB=s,this._$AM=i,this.options=r,this._$Cv=r?.isConnected??!0}get parentNode(){let t=this._$AA.parentNode;const s=this._$AM;return s!==void 0&&t?.nodeType===11&&(t=s.parentNode),t}get startNode(){return this._$AA}get endNode(){return this._$AB}_$AI(t,s=this){t=ze(this,t,s),ss(t)?t===A||t==null||t===""?(this._$AH!==A&&this._$AR(),this._$AH=A):t!==this._$AH&&t!==Pe&&this._(t):t._$litType$!==void 0?this.$(t):t.nodeType!==void 0?this.T(t):ho(t)?this.k(t):this._(t)}O(t){return this._$AA.parentNode.insertBefore(t,this._$AB)}T(t){this._$AH!==t&&(this._$AR(),this._$AH=this.O(t))}_(t){this._$AH!==A&&ss(this._$AH)?this._$AA.nextSibling.data=t:this.T(Se.createTextNode(t)),this._$AH=t}$(t){const{values:s,_$litType$:i}=t,r=typeof i=="number"?this._$AC(t):(i.el===void 0&&(i.el=is.createElement(qn(i.h,i.h[0]),this.options)),i);if(this._$AH?._$AD===r)this._$AH.p(s);else{const n=new lo(r,this),a=n.u(this.options);n.p(s),this.T(a),this._$AH=n}}_$AC(t){let s=Sr.get(t.strings);return s===void 0&&Sr.set(t.strings,s=new is(t)),s}k(t){Ki(this._$AH)||(this._$AH=[],this._$AR());const s=this._$AH;let i,r=0;for(const n of t)r===s.length?s.push(i=new He(this.O(es()),this.O(es()),this,this.options)):i=s[r],i._$AI(n),r++;r<s.length&&(this._$AR(i&&i._$AB.nextSibling,r),s.length=r)}_$AR(t=this._$AA.nextSibling,s){for(this._$AP?.(!1,!0,s);t!==this._$AB;){const i=br(t).nextSibling;br(t).remove(),t=i}}setConnected(t){this._$AM===void 0&&(this._$Cv=t,this._$AP?.(t))}}class Ns{get tagName(){return this.element.tagName}get _$AU(){return this._$AM._$AU}constructor(t,s,i,r,n){this.type=1,this._$AH=A,this._$AN=void 0,this.element=t,this.name=s,this._$AM=r,this.options=n,i.length>2||i[0]!==""||i[1]!==""?(this._$AH=Array(i.length-1).fill(new String),this.strings=i):this._$AH=A}_$AI(t,s=this,i,r){const n=this.strings;let a=!1;if(n===void 0)t=ze(this,t,s,0),a=!ss(t)||t!==this._$AH&&t!==Pe,a&&(this._$AH=t);else{const o=t;let h,c;for(t=n[0],h=0;h<n.length-1;h++)c=ze(this,o[i+h],s,h),c===Pe&&(c=this._$AH[h]),a||=!ss(c)||c!==this._$AH[h],c===A?t=A:t!==A&&(t+=(c??"")+n[h+1]),this._$AH[h]=c}a&&!r&&this.j(t)}j(t){t===A?this.element.removeAttribute(this.name):this.element.setAttribute(this.name,t??"")}}class uo extends Ns{constructor(){super(...arguments),this.type=3}j(t){this.element[this.name]=t===A?void 0:t}}class po extends Ns{constructor(){super(...arguments),this.type=4}j(t){this.element.toggleAttribute(this.name,!!t&&t!==A)}}class fo extends Ns{constructor(t,s,i,r,n){super(t,s,i,r,n),this.type=5}_$AI(t,s=this){if((t=ze(this,t,s,0)??A)===Pe)return;const i=this._$AH,r=t===A&&i!==A||t.capture!==i.capture||t.once!==i.once||t.passive!==i.passive,n=t!==A&&(i===A||r);r&&this.element.removeEventListener(this.name,this,i),n&&this.element.addEventListener(this.name,this,t),this._$AH=t}handleEvent(t){typeof this._$AH=="function"?this._$AH.call(this.options?.host??this.element,t):this._$AH.handleEvent(t)}}class _o{constructor(t,s,i){this.element=t,this.type=6,this._$AN=void 0,this._$AM=s,this.options=i}get _$AU(){return this._$AM._$AU}_$AI(t){ze(this,t)}}const mo={I:He},go=qi.litHtmlPolyfillSupport;go?.(is,He),(qi.litHtmlVersions??=[]).push("3.3.2");const yo=(e,t,s)=>{const i=s?.renderBefore??t;let r=i._$litPart$;if(r===void 0){const n=s?.renderBefore??null;i._$litPart$=r=new He(t.insertBefore(es(),n),n,void 0,s??{})}return r._$AI(e),r};/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const Zi=globalThis;let Bt=class extends De{constructor(){super(...arguments),this.renderOptions={host:this},this._$Do=void 0}createRenderRoot(){const t=super.createRenderRoot();return this.renderOptions.renderBefore??=t.firstChild,t}update(t){const s=this.render();this.hasUpdated||(this.renderOptions.isConnected=this.isConnected),super.update(t),this._$Do=yo(s,this.renderRoot,this.renderOptions)}connectedCallback(){super.connectedCallback(),this._$Do?.setConnected(!0)}disconnectedCallback(){super.disconnectedCallback(),this._$Do?.setConnected(!1)}render(){return Pe}};Bt._$litElement$=!0,Bt.finalized=!0,Zi.litElementHydrateSupport?.({LitElement:Bt});const vo=Zi.litElementPolyfillSupport;vo?.({LitElement:Bt});(Zi.litElementVersions??=[]).push("4.2.2");/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const de=e=>(t,s)=>{s!==void 0?s.addInitializer(()=>{customElements.define(e,t)}):customElements.define(e,t)};/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const bo={attribute:!0,type:String,converter:Ps,reflect:!1,hasChanged:Gi},wo=(e=bo,t,s)=>{const{kind:i,metadata:r}=s;let n=globalThis.litPropertyMetadata.get(r);if(n===void 0&&globalThis.litPropertyMetadata.set(r,n=new Map),i==="setter"&&((e=Object.create(e)).wrapped=!0),n.set(s.name,e),i==="accessor"){const{name:a}=s;return{set(o){const h=t.get.call(this);t.set.call(this,o),this.requestUpdate(a,h,e,!0,o)},init(o){return o!==void 0&&this.C(a,void 0,e,o),o}}}if(i==="setter"){const{name:a}=s;return function(o){const h=this[a];t.call(this,o),this.requestUpdate(a,h,e,!0,o)}}throw Error("Unsupported decorator location: "+i)};function Qi(e){return(t,s)=>typeof s=="object"?wo(e,t,s):((i,r,n)=>{const a=r.hasOwnProperty(n);return r.constructor.createProperty(n,i),a?Object.getOwnPropertyDescriptor(r,n):void 0})(e,t,s)}/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */function T(e){return Qi({...e,state:!0,attribute:!1})}/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const xo=(e,t,s)=>(s.configurable=!0,s.enumerable=!0,Reflect.decorate&&typeof t!="object"&&Object.defineProperty(e,t,s),s);/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */function as(e,t){return(s,i,r)=>{const n=a=>a.renderRoot?.querySelector(e)??null;return xo(s,i,{get(){return n(this)}})}}/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */let Kn=class extends Event{constructor(t,s,i,r){super("context-request",{bubbles:!0,composed:!0}),this.context=t,this.contextTarget=s,this.callback=i,this.subscribe=r??!1}};/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 *//**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */let Me=class{constructor(t,s,i,r){if(this.subscribe=!1,this.provided=!1,this.value=void 0,this.t=(n,a)=>{this.unsubscribe&&(this.unsubscribe!==a&&(this.provided=!1,this.unsubscribe()),this.subscribe||this.unsubscribe()),this.value=n,this.host.requestUpdate(),this.provided&&!this.subscribe||(this.provided=!0,this.callback&&this.callback(n,a)),this.unsubscribe=a},this.host=t,s.context!==void 0){const n=s;this.context=n.context,this.callback=n.callback,this.subscribe=n.subscribe??!1}else this.context=s,this.callback=i,this.subscribe=r??!1;this.host.addController(this)}hostConnected(){this.dispatchRequest()}hostDisconnected(){this.unsubscribe&&(this.unsubscribe(),this.unsubscribe=void 0)}dispatchRequest(){this.host.dispatchEvent(new Kn(this.context,this.host,this.t,this.subscribe))}};/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */let ko=class{get value(){return this.o}set value(t){this.setValue(t)}setValue(t,s=!1){const i=s||!Object.is(t,this.o);this.o=t,i&&this.updateObservers()}constructor(t){this.subscriptions=new Map,this.updateObservers=()=>{for(const[s,{disposer:i}]of this.subscriptions)s(this.o,i)},t!==void 0&&(this.value=t)}addCallback(t,s,i){if(!i)return void t(this.value);this.subscriptions.has(t)||this.subscriptions.set(t,{disposer:()=>{this.subscriptions.delete(t)},consumerHost:s});const{disposer:r}=this.subscriptions.get(t);t(this.value,r)}clearCallbacks(){this.subscriptions.clear()}};/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */let Mo=class extends Event{constructor(t,s){super("context-provider",{bubbles:!0,composed:!0}),this.context=t,this.contextTarget=s}},Qs=class extends ko{constructor(t,s,i){super(s.context!==void 0?s.initialValue:i),this.onContextRequest=r=>{if(r.context!==this.context)return;const n=r.contextTarget??r.composedPath()[0];n!==this.host&&(r.stopPropagation(),this.addCallback(r.callback,n,r.subscribe))},this.onProviderRequest=r=>{if(r.context!==this.context||(r.contextTarget??r.composedPath()[0])===this.host)return;const n=new Set;for(const[a,{consumerHost:o}]of this.subscriptions)n.has(a)||(n.add(a),o.dispatchEvent(new Kn(this.context,o,a,!0)));r.stopPropagation()},this.host=t,s.context!==void 0?this.context=s.context:this.context=s,this.attachListeners(),this.host.addController?.(this)}attachListeners(){this.host.addEventListener("context-request",this.onContextRequest),this.host.addEventListener("context-provider",this.onProviderRequest)}hostConnected(){this.host.dispatchEvent(new Mo(this.context,this.host))}};const Fe="drawing-context",Co=["round","flat","chisel","calligraphy","fan","splatter","image"];function So(e){return typeof e=="string"&&Co.includes(e)}const xi={multiply:"Multiply",subtract:"Subtract",height:"Height"};function Po(e){return typeof e=="string"&&Object.prototype.hasOwnProperty.call(xi,e)}const ki={normal:"Normal",multiply:"Multiply",screen:"Screen",overlay:"Overlay",darken:"Darken",lighten:"Lighten","color-burn":"Color Burn","color-dodge":"Color Dodge",hue:"Hue",saturation:"Saturation",color:"Color",luminosity:"Luminosity",behind:"Behind",clear:"Clear"};function Zn(e){return typeof e=="string"&&Object.prototype.hasOwnProperty.call(ki,e)}const To={normal:"Normal",multiply:"Multiply",screen:"Screen",overlay:"Overlay",darken:"Darken",lighten:"Lighten","soft-light":"Soft Light"},$o={normal:"source-over",multiply:"multiply",screen:"screen",overlay:"overlay",darken:"darken",lighten:"lighten","soft-light":"soft-light"};function Qe(e){return $o[e]}const Io={linear:e=>e,light:e=>Math.pow(e,.5),heavy:e=>Math.pow(e,2)};function Eo(e){return Math.max(2,Math.round(e/2)*2)}class te{constructor(t){this._state=t>>>0}next(){this._state=this._state+1831565813>>>0;let t=this._state;return t=Math.imul(t^t>>>15,t|1),t^=t+Math.imul(t^t>>>7,t|61),((t^t>>>14)>>>0)/4294967296}}function Ro(){return Math.floor(Math.random()*4294967296)>>>0}const et=256,Xs=new Map,$s=new Map,Ji=[["paper","Paper"],["canvas","Canvas"],["noise","Noise"],["charcoal","Charcoal"]],Qn=new Set(Ji.map(([e])=>e)),Pr=new Map,tr=100;function Tr(e,t,s,i,r,n){if(!e||e.length>tr||e==="none"||Qn.has(e)||!Number.isInteger(s)||!Number.isInteger(i)||s<1||i<1||r.length!==s*i)return!1;if(Xs.set(e,{id:e,label:t,width:s,height:i,data:r}),n){const a=$s.get(e);a?a.add(n):$s.set(e,new Set([n]))}return!0}function Js(e,t){if(t){const s=$s.get(e);if(s&&(s.delete(t),s.size>0))return}$s.delete(e),Xs.delete(e)}function Do(e){const t=Xs.get(e);if(t)return t;if(!Qn.has(e))return null;let s=Pr.get(e);return s||(s={id:e,label:Ji.find(([i])=>i===e)[1],width:et,height:et,data:Oo(e)},Pr.set(e,s)),s}function Ao(){return[...Ji.map(([e,t])=>({id:e,label:t})),...[...Xs.values()].map(e=>({id:e.id,label:e.label}))]}function Lo(e){if(!e||!(e.depth>0)||e.pattern==="none")return null;const t=Do(e.pattern);return t?{pattern:t,scale:e.scale>0?e.scale:1,depth:e.depth,mode:e.mode,invert:e.invert,depthPressure:e.depthPressure,perTip:e.perTip}:null}function hs(e,t,s){let i=Math.imul(e,374761393)^Math.imul(t,668265263)^Math.imul(s,1274126177);return i=Math.imul(i^i>>>13,1274126177),i^=i>>>16,(i>>>0)/4294967296}function zt(e,t,s,i,r){for(let n=0;n<et;n++){const a=n*s/et,o=Math.floor(a),h=a-o,c=h*h*(3-2*h),l=o%s,d=(o+1)%s;for(let u=0;u<et;u++){const _=u*t/et,f=Math.floor(_),m=_-f,p=m*m*(3-2*m),g=f%t,y=(f+1)%t,w=hs(g,l,i),x=hs(y,l,i),b=hs(g,d,i),S=hs(y,d,i);e[n*et+u]+=r*((w+(x-w)*p)*(1-c)+(b+(S-b)*p)*c)}}}function cs(e){let s=1/0,i=-1/0;for(const l of e)l<s&&(s=l),l>i&&(i=l);const r=i-s||1,n=new Uint32Array(4096),a=l=>Math.min(4095,Math.floor((l-s)/r*4096));for(const l of e)n[a(l)]++;const o=new Float32Array(4096);let h=0;for(let l=0;l<4096;l++)o[l]=h+n[l]/2,h+=n[l];const c=new Uint8Array(e.length);for(let l=0;l<e.length;l++)c[l]=Math.round(o[a(e[l])]/e.length*255);return c}function jo(e,t,s,i,r,n){const a=Math.cos(i),o=Math.sin(i);for(let h=0;h<r;h++){const c=(Math.round(t+a*h)%et+et)%et,l=(Math.round(s+o*h)%et+et)%et;e[l*et+c]+=n}}function zo(e,t,s){const i=Math.max(0,Math.min(1,(s-e)/(t-e)));return i*i*(3-2*i)}function Oo(e){const t=new Float32Array(et*et);switch(e){case"noise":return zt(t,16,16,11,.5),zt(t,32,32,12,.3),zt(t,64,64,13,.15),zt(t,128,128,14,.05),cs(t);case"paper":{zt(t,8,8,21,.2),zt(t,32,32,22,.3),zt(t,128,128,23,.3),zt(t,256,256,24,.2);const s=new te(10133010);for(let i=0;i<700;i++)jo(t,s.next()*et,s.next()*et,s.next()*Math.PI,6+s.next()*18,.22);return cs(t)}case"canvas":{zt(t,128,128,31,.12);for(let i=0;i<et;i++)for(let r=0;r<et;r++){const n=r%8/8,a=i%8/8,o=Math.sin(Math.PI*n),h=Math.sin(Math.PI*a),c=Math.floor(r/8)+Math.floor(i/8)&1;t[i*et+r]+=c?h*(.7+.3*o):o*(.7+.3*h)}return cs(t)}case"charcoal":{zt(t,128,128,41,.45),zt(t,64,64,42,.2),zt(t,16,16,43,.35);const s=cs(t);for(let i=0;i<s.length;i++)s[i]=Math.round(zo(.2,.8,s[i]/255)*255);return s}}return new Uint8Array(et*et).fill(255)}function $r(e,t,s,i,r,n,a=n.depth){if(!(a>0))return;const{data:o,width:h,height:c}=n.pattern,{scale:l,mode:d,invert:u}=n,_=new Int32Array(t),f=new Int32Array(t),m=new Float32Array(t);for(let p=0;p<t;p++){const g=(i+p+.5)/l-.5,y=Math.floor(g);m[p]=g-y,_[p]=(y%h+h)%h,f[p]=(_[p]+1)%h}for(let p=0;p<s;p++){const g=(r+p+.5)/l-.5,y=Math.floor(g),w=g-y,x=(y%c+c)%c*h,b=((y+1)%c+c)%c*h;for(let S=0;S<t;S++){const E=(p*t+S)*4,D=e[E+3];if(D===0)continue;const L=m[S],I=o[x+_[S]]+(o[x+f[S]]-o[x+_[S]])*L,k=o[b+_[S]]+(o[b+f[S]]-o[b+_[S]])*L;let M=(I+(k-I)*w)/255;u&&(M=1-M);const P=a*(1-M);let R;d==="multiply"?R=D*(1-P):d==="subtract"?R=Math.max(0,D-P*255):R=P>=1?0:Math.max(0,(D/255-P)/(1-P))*255,R!==D&&(e[E+3]=Math.round(R))}}}const Bo={round:{aspect:!1,rotation:!1,bristles:!1,hardness:!0},flat:{aspect:!0,rotation:!0,bristles:!1,hardness:!0},chisel:{aspect:!0,rotation:!0,bristles:!1,hardness:!0},calligraphy:{aspect:!0,rotation:!0,bristles:!1,hardness:!0},fan:{aspect:!1,rotation:!0,bristles:!0,hardness:!0},splatter:{aspect:!1,rotation:!0,bristles:!0,hardness:!0},image:{aspect:!1,rotation:!0,bristles:!1,hardness:!1}},Ho={round:{shape:"round",aspect:1,angle:0,orientation:"fixed"},flat:{shape:"flat",aspect:3,angle:0,orientation:"direction"},chisel:{shape:"chisel",aspect:2.5,angle:0,orientation:"direction"},calligraphy:{shape:"calligraphy",aspect:4,angle:45,orientation:"fixed"},fan:{shape:"fan",aspect:1,angle:0,orientation:"direction",bristles:8,spread:120},splatter:{shape:"splatter",aspect:1,angle:0,orientation:"fixed",bristles:12,spread:.8},image:{shape:"image",aspect:1,angle:0,orientation:"fixed"}};function Fo(e){return Bo[e]}function Q(e){return{...Ho[e]}}const pe={depletion:0,depletionLength:500,buildup:0,wetness:0},bt={sizeVelocity:0,flowVelocity:0,minSize:0,tilt:0},It={size:0,angle:0,roundness:0,flow:0},er=5,sr=16,Et={amount:0,bothAxes:!1,count:1,countJitter:0},Jn=5,ta=60,wt=20;function ir(e){return typeof e=="number"&&Number.isFinite(e)?Math.max(Jn,Math.min(ta,e)):wt}const ea=5e3,yt={sizeLength:0,sizeUnit:"distance",taperStart:0};function No(e){const t={...yt};for(const s of["sizeLength","taperStart"]){const i=e?.[s];typeof i=="number"&&Number.isFinite(i)&&(t[s]=Math.max(0,Math.min(ea,i)))}return e?.sizeUnit==="dabs"&&(t.sizeUnit="dabs"),t}const sa=.1,ia=4,$t={pattern:"none",scale:1,depth:.8,mode:"multiply",invert:!1,depthPressure:!1,perTip:!1};function ra(e){const t={...$t};return!e||typeof e!="object"||(typeof e.pattern=="string"&&e.pattern.length>0&&e.pattern.length<=tr&&(t.pattern=e.pattern),typeof e.scale=="number"&&Number.isFinite(e.scale)&&(t.scale=Math.max(sa,Math.min(ia,e.scale))),typeof e.depth=="number"&&Number.isFinite(e.depth)&&(t.depth=Math.max(0,Math.min(1,e.depth))),Po(e.mode)&&(t.mode=e.mode),t.invert=e.invert===!0,t.depthPressure=e.depthPressure===!0,t.perTip=e.perTip===!0),t}const xt={hue:0,saturation:0,brightness:0,purity:0,fgBg:0,fgBgPressure:!1,perTip:!0};function rr(e){return!!e&&(e.hue>0||e.saturation>0||e.brightness>0||e.purity!==0||e.fgBg>0)}const Je=[{id:"round",name:"Round",category:"basic",descriptor:{size:4,opacity:1,flow:1,hardness:1,spacing:.15,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:Q("round"),ink:{...pe},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"ink-pen",name:"Ink Pen",category:"basic",descriptor:{size:6,opacity:1,flow:1,hardness:1,spacing:.1,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:Q("round"),ink:{...pe},dynamics:{...bt,minSize:.15},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt,taperStart:40},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"soft-round",name:"Soft Round",category:"basic",descriptor:{size:20,opacity:1,flow:.6,hardness:.3,spacing:.12,pressureSize:!0,pressureOpacity:!0,pressureCurve:"light",tip:Q("round"),ink:{...pe},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"flat",name:"Flat",category:"artistic",descriptor:{size:30,opacity:1,flow:.8,hardness:.9,spacing:.1,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:Q("flat"),ink:{depletion:.3,depletionLength:800,buildup:0,wetness:0},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"chisel",name:"Chisel",category:"artistic",descriptor:{size:24,opacity:1,flow:.9,hardness:.95,spacing:.1,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:Q("chisel"),ink:{depletion:.2,depletionLength:600,buildup:.3,wetness:0},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"calligraphy",name:"Calligraphy",category:"artistic",descriptor:{size:20,opacity:1,flow:1,hardness:1,spacing:.08,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:Q("calligraphy"),ink:{...pe},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"fan",name:"Fan",category:"artistic",descriptor:{size:40,opacity:1,flow:.7,hardness:.8,spacing:.15,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:Q("fan"),ink:{depletion:.5,depletionLength:600,buildup:0,wetness:0},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"splatter",name:"Splatter",category:"effects",descriptor:{size:50,opacity:.8,flow:.6,hardness:.7,spacing:.25,pressureSize:!1,pressureOpacity:!0,pressureCurve:"linear",tip:Q("splatter"),ink:{depletion:.7,depletionLength:400,buildup:0,wetness:0},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"spray",name:"Spray",category:"effects",descriptor:{size:12,opacity:1,flow:.6,hardness:.5,spacing:.5,pressureSize:!1,pressureOpacity:!1,pressureCurve:"linear",tip:Q("round"),ink:{...pe},dynamics:{...bt},jitter:{size:.6,angle:0,roundness:0,flow:.5},scatter:{amount:2,bothAxes:!0,count:8,countJitter:.5},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"dry-brush",name:"Dry Brush",category:"artistic",descriptor:{size:25,opacity:1,flow:.5,hardness:.8,spacing:.12,pressureSize:!0,pressureOpacity:!0,pressureCurve:"heavy",tip:Q("round"),ink:{depletion:.8,depletionLength:300,buildup:.4,wetness:0},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"wet-brush",name:"Wet Brush",category:"artistic",descriptor:{size:20,opacity:.8,flow:.7,hardness:.5,spacing:.1,pressureSize:!1,pressureOpacity:!1,pressureCurve:"linear",tip:Q("round"),ink:{depletion:0,depletionLength:500,buildup:.2,wetness:.91},dynamics:{...bt},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"confetti",name:"Confetti",category:"effects",descriptor:{size:14,opacity:1,flow:1,hardness:1,spacing:.9,pressureSize:!1,pressureOpacity:!1,pressureCurve:"linear",tip:Q("flat"),ink:{...pe},dynamics:{...bt},jitter:{size:.4,angle:1,roundness:.5,flow:0},scatter:{amount:1.5,bothAxes:!0,count:3,countJitter:.3},color:{hue:1,saturation:.3,brightness:.2,purity:0,fgBg:1,fgBgPressure:!1,perTip:!0},fade:{...yt},texture:{...$t},blendMode:"normal",airbrush:!1,airbrushRate:wt}},{id:"charcoal",name:"Charcoal",category:"artistic",descriptor:{size:30,opacity:1,flow:.8,hardness:.6,spacing:.1,pressureSize:!0,pressureOpacity:!0,pressureCurve:"linear",tip:Q("round"),ink:{...pe},dynamics:{...bt,minSize:.4},jitter:{...It},scatter:{...Et},color:{...xt},fade:{...yt},texture:{pattern:"charcoal",scale:1,depth:.9,mode:"height",invert:!1,depthPressure:!1,perTip:!1},blendMode:"normal",airbrush:!1,airbrushRate:wt}}];function Xo(e){return Je.find(t=>t.id===e)}function Is(){return Oe(Je[0].descriptor)}function Oe(e){return{...e,tip:{...e.tip},ink:{...e.ink},dynamics:{...e.dynamics},jitter:{...It,...e.jitter},scatter:{...Et,...e.scatter},color:{...xt,...e.color},fade:{...yt,...e.fade},texture:ra(e.texture),blendMode:Zn(e.blendMode)?e.blendMode:"normal",airbrush:e.airbrush===!0,airbrushRate:ir(e.airbrushRate)}}const Wo={sizeVelocity:[-1,1],flowVelocity:[-1,1],minSize:[0,1],tilt:[0,1]};function Yo(e){const t={...bt};for(const s of Object.keys(t)){const i=e?.[s],[r,n]=Wo[s];typeof i=="number"&&Number.isFinite(i)&&(t[s]=Math.max(r,Math.min(n,i)))}return t}function Uo(e){const t={...It};for(const s of Object.keys(t)){const i=e?.[s];typeof i=="number"&&Number.isFinite(i)&&(t[s]=Math.max(0,Math.min(1,i)))}return t}function Vo(e){const t={...Et},s=(i,r,n,a)=>typeof i=="number"&&Number.isFinite(i)?Math.max(r,Math.min(n,i)):a;return t.amount=s(e?.amount,0,er,t.amount),t.count=Math.round(s(e?.count,1,sr,t.count)),t.countJitter=s(e?.countJitter,0,1,t.countJitter),typeof e?.bothAxes=="boolean"&&(t.bothAxes=e.bothAxes),t}function Go(e){const t={...xt},s=(i,r,n,a)=>typeof i=="number"&&Number.isFinite(i)?Math.max(r,Math.min(n,i)):a;return t.hue=s(e?.hue,0,1,t.hue),t.saturation=s(e?.saturation,0,1,t.saturation),t.brightness=s(e?.brightness,0,1,t.brightness),t.purity=s(e?.purity,-1,1,t.purity),t.fgBg=s(e?.fgBg,0,1,t.fgBg),typeof e?.fgBgPressure=="boolean"&&(t.fgBgPressure=e.fgBgPressure),typeof e?.perTip=="boolean"&&(t.perTip=e.perTip),t}const qo=128;function Ko(e){const t=Q("round");if(!e||typeof e!="object"||!So(e.shape))return t;const{imageId:s,...i}=e;return e.shape==="image"?typeof s!="string"||s.length===0||s.length>qo?t:{...Q("image"),...i,imageId:s}:{...t,...i}}function Ws(e){const t=Is(),s=e??{};return{size:s.size??t.size,opacity:s.opacity??t.opacity,flow:s.flow??t.flow,hardness:s.hardness??t.hardness,spacing:s.spacing??t.spacing,pressureSize:s.pressureSize??t.pressureSize,pressureOpacity:s.pressureOpacity??t.pressureOpacity,pressureCurve:s.pressureCurve??t.pressureCurve,tip:Ko(s.tip),ink:{...t.ink,...s.ink??{}},dynamics:Yo(s.dynamics),jitter:Uo(s.jitter),scatter:Vo(s.scatter),color:Go(s.color),fade:No(s.fade),texture:ra(s.texture),blendMode:Zn(s.blendMode)?s.blendMode:"normal",airbrush:s.airbrush===!0,airbrushRate:ir(s.airbrushRate)}}function jt(){if(typeof crypto<"u"&&typeof crypto.randomUUID=="function")return crypto.randomUUID();const e=new Uint8Array(16);crypto.getRandomValues(e),e[6]=e[6]&15|64,e[8]=e[8]&63|128;const t=Array.from(e,s=>s.toString(16).padStart(2,"0")).join("");return`${t.slice(0,8)}-${t.slice(8,12)}-${t.slice(12,16)}-${t.slice(16,20)}-${t.slice(20)}`}const na=1024,Zo=48,Qo=3;function Jo(e,t,s){const i=new Uint8Array(t*s);for(let r=0,n=0;r<i.length;r++,n+=4){const a=.2126*e[n]+.7152*e[n+1]+.0722*e[n+2];i[r]=Math.round((255-a)*e[n+3]/255)}return i}function aa(e){const{width:t,height:s,alpha:i}=e;let r=t,n=s,a=-1,o=-1;for(let d=0;d<s;d++){const u=d*t;for(let _=0;_<t;_++)i[u+_]>Qo&&(_<r&&(r=_),_>a&&(a=_),d<n&&(n=d),d>o&&(o=d))}if(a<0)return null;const h=a-r+1,c=o-n+1;if(h===t&&c===s)return e;const l=new Uint8Array(h*c);for(let d=0;d<c;d++)l.set(i.subarray((n+d)*t+r,(n+d)*t+r+h),d*h);return{width:h,height:c,alpha:l}}function Ys(e,t){const s=Math.max(e.width,e.height);if(s<=t)return e;const i=t/s,r=Math.max(1,Math.round(e.width*i)),n=Math.max(1,Math.round(e.height*i)),a=e.width/r,o=e.height/n,h=new Float32Array(r*e.height);for(let l=0;l<e.height;l++)for(let d=0;d<r;d++)h[l*r+d]=Ir(e.alpha,l*e.width,1,d*a,(d+1)*a,e.width);const c=new Uint8Array(r*n);for(let l=0;l<n;l++)for(let d=0;d<r;d++)c[l*r+d]=Math.round(Ir(h,d,r,l*o,(l+1)*o,e.height));return{width:r,height:n,alpha:c}}function Ir(e,t,s,i,r,n){let a=0,o=0;const h=Math.floor(i),c=Math.min(n-1,Math.ceil(r)-1);for(let l=h;l<=c;l++){const d=Math.min(l+1,r)-Math.max(l,i);d<=0||(a+=e[t+l*s]*d,o+=d)}return o>0?a/o:0}function th(e,t,s){if(!(t>0&&s>0))return null;const i=aa({width:t,height:s,alpha:Jo(e,t,s)});return i&&Ys(i,na)}function Er(e){return Ys(e,Zo)}function eh(e){return Object.prototype.toString.call(e)==="[object Uint8Array]"}function ts(e){const t=e;return!!t&&Number.isInteger(t.width)&&Number.isInteger(t.height)&&t.width>0&&t.height>0&&t.width<=8192&&t.height<=8192&&eh(t.alpha)&&t.alpha.length===t.width*t.height}function kt(e,t){if(typeof OffscreenCanvas<"u")return new OffscreenCanvas(e,t);const s=document.createElement("canvas");return s.width=e,s.height=t,s}function ot(e,t){return e.getContext("2d",t)}function ls(e,t,s,i,r,n){r!==void 0&&n!==void 0?e.drawImage(t,s,i,r,n):e.drawImage(t,s,i)}function Mi(e,t,s,i){e.globalCompositeOperation="source-in",e.fillStyle=t,e.fillRect(0,0,s,i),e.globalCompositeOperation="source-over"}const Lt=new Map,rs=new Map,Ue=new Map;let sh=0;const nr=e=>e?Lt.get(e)??Ue.get(e)?.entry:void 0;function ih(e){const t=e?Lt.get(e):void 0;if(!e||!t)return()=>{};const s=Ue.get(e);s&&s.entry===t?s.count++:Ue.set(e,{entry:t,count:1});let i=!1;return()=>{if(i)return;i=!0;const r=Ue.get(e);r&&--r.count<=0&&Ue.delete(e)}}const rh=64*1024*1024;let ws=0;const Ci=e=>e.reduce((t,s)=>t+s.width*s.height*4,0);function ti(e,t,s){if(s&&Lt.has(e)){oa(e,s);return}ha(e),Lt.set(e,{mask:t,revision:++sh,levels:null}),s&&rs.set(e,new Set([s]))}function oa(e,t){if(!Lt.has(e))return;const s=rs.get(e);s?s.add(t):rs.set(e,new Set([t]))}function Rr(e,t){if(t){const s=rs.get(e);if(s&&(s.delete(t),s.size>0))return}ha(e)}function ha(e){const t=Lt.get(e);t?.levels&&(ws-=Ci(t.levels)),Lt.delete(e),rs.delete(e)}function ca(e){return!!e&&Lt.has(e)}function xs(e){return nr(e)?.revision||0}function nh(e){return nr(e)?.mask??null}function la(e){return e.shape==="image"&&!ca(e.imageId)}function ah(e){return la(e)?{...Q("round"),orientation:e.orientation,angle:e.angle}:e}function da(e,t){const s=Math.max(1,e),i=nh(t.imageId);return i?i.width>=i.height?{w:s,h:Math.max(1,s*i.height/i.width)}:{w:Math.max(1,s*i.width/i.height),h:s}:{w:s,h:s}}function oh(e,t){if(t.levels)return Lt.get(e)===t&&(Lt.delete(e),Lt.set(e,t)),t.levels;const{mask:s}=t,i=kt(s.width,s.height),r=ot(i),n=r.createImageData(s.width,s.height);for(let h=0,c=0;h<s.alpha.length;h++,c+=4)n.data[c]=n.data[c+1]=n.data[c+2]=255,n.data[c+3]=s.alpha[h];r.putImageData(n,0,0);const a=[i];let o=i;for(;Math.max(o.width,o.height)>16;){const h=Math.max(1,Math.ceil(o.width/2)),c=Math.max(1,Math.ceil(o.height/2)),l=kt(h,c),d=ot(l);d.imageSmoothingEnabled=!0,d.imageSmoothingQuality="high",d.drawImage(o,0,0,h,c),a.push(l),o=l}if(t.levels=a,Lt.get(e)!==t)return a;ws+=Ci(a);for(const h of Lt.values()){if(ws<=rh)break;h===t||!h.levels||(ws-=Ci(h.levels),h.levels=null)}return a}function hh(e,t,s){const i=nr(s.imageId),{w:r,h:n}=da(Math.max(1,e),s),a=Math.max(1,Math.round(r)),o=Math.max(1,Math.round(n)),h=kt(a,o),c=ot(h);if(!i)return c.fillStyle="#fff",c.beginPath(),c.arc(a/2,o/2,a/2,0,Math.PI*2),c.fill(),h;const l=oh(s.imageId,i);let d=l[0];for(const u of l)Math.max(u.width,u.height)>=e&&(d=u);return c.imageSmoothingEnabled=!0,c.imageSmoothingQuality="high",c.drawImage(d,0,0,a,o),h}const ch=.25;function lh(e){return{id:e.id,name:e.name,category:"custom",descriptor:Ws(e.descriptor),...e.group?{group:e.group}:{}}}class dh{constructor(t,s=()=>{},i=()=>{}){this._store=t,this._onChange=s,this._onStored=i,this.tips=[],this.presets=[],this._loading=new Map,this._deleted=new Map,this._known=new Set,this._failed=new Set,this._knownPatterns=new Set,this._changes=0,this._released=!1,this._releases=0}async load(){for(let t=0;t<3;t++){const s=this._changes,i=this._releases,{tips:r,presets:n}=await this._store.list();let a=null;try{a=await this._store.listPatterns()}catch(h){console.error("Could not load brush patterns:",h)}if(i!==this._releases)return;if(s!==this._changes)continue;const o=new Set(r.map(h=>h.id));for(const h of this._known)o.has(h)||this._forgetTip(h);if(this._known=o,a){const h=new Set(a.map(c=>c.id));for(const c of this._knownPatterns)h.has(c)||Js(c,this);for(const c of a)Tr(c.id,c.name,c.width,c.height,c.data,this);this._knownPatterns=h}this._released=!1,this._failed.clear(),this.tips=r,this.presets=n,this._onChange();return}}get released(){return this._released}release(){this._released=!0,this._releases++;for(const t of this._known)Rr(t,this);for(const t of this._knownPatterns)Js(t,this)}reload(){return this.load().catch(t=>console.error("Could not reload custom brushes:",t))}tipMeta(t){return this.tips.find(s=>s.id===t)}ensureTip(t){if(this._released)return Promise.resolve(!1);if(ca(t))return this.tipMeta(t)&&oa(t,this),Promise.resolve(!0);if(this._failed.has(t))return Promise.resolve(!1);const s=this._loading.get(t);if(s)return s;const i=this._deleted.get(t)??0,r=(async()=>{try{const n=await this._store.getMask(t);return!n||!ts(n)?(this._failed.add(t),!1):(this._deleted.get(t)??0)!==i||this._released?!1:(ti(t,n,this),this._onChange(),!0)}catch(n){return console.error("Could not load brush tip:",n),this._failed.add(t),!1}finally{this._loading.delete(t)}})();return this._loading.set(t,r),r}async add(t){const s=Date.now(),i=[],r=[];t.forEach((n,a)=>{if(!ts(n.mask))throw new RangeError("Not a valid tip mask");const o=jt(),h=n.spacing??ch,c={id:o,name:n.name,width:n.mask.width,height:n.mask.height,spacing:h,createdAt:s+a,thumb:Er(n.mask)},l=Ws(n.descriptor),d=Oe({...l,spacing:h,tip:{...Q("image"),imageId:o}});i.push({meta:c,mask:n.mask}),r.push({id:jt(),name:n.name,descriptor:d,tipId:o,createdAt:s+a})}),await this._store.putBatch({tips:i,presets:r});for(const n of i)this._released||ti(n.meta.id,n.mask,this),this._known.add(n.meta.id);return this.tips=[...this.tips,...i.map(n=>n.meta)],this.presets=[...this.presets,...r],this._changes++,this._onChange(),this._onStored(),r}async importBrushes(t){const s=Date.now(),i=[],r=new Map;t.tips.forEach((o,h)=>{if(!ts(o.mask))throw new RangeError("Not a valid tip mask");const c=jt();r.set(o.key,c),i.push({meta:{id:c,name:o.name,width:o.mask.width,height:o.mask.height,spacing:o.spacing,createdAt:s+h,thumb:Er(o.mask),group:t.group},mask:o.mask})});const n=t.brushes.map((o,h)=>{const c=o.tipKey?r.get(o.tipKey):void 0,l=Oe(c?{...o.descriptor,tip:{...o.descriptor.tip,shape:"image",imageId:c}}:o.descriptor);return{id:jt(),name:o.name,descriptor:l,...c?{tipId:c}:{},createdAt:s+h,group:t.group}}),a=t.patterns.map((o,h)=>({...o,createdAt:s+h}));await this._store.putBatch({tips:i,presets:n,patterns:a});for(const o of i)this._released||ti(o.meta.id,o.mask,this),this._known.add(o.meta.id);for(const o of a)this._released||Tr(o.id,o.name,o.width,o.height,o.data,this),this._knownPatterns.add(o.id);return this.tips=[...this.tips,...i.map(o=>o.meta)],this.presets=[...this.presets,...n],this._changes++,this._onChange(),this._onStored(),n.length}groups(){return[...new Set(this.presets.flatMap(t=>t.group?[t.group]:[]))]}async removeGroup(t){const s=this.presets.filter(c=>c.group===t).map(c=>c.id),i=new Set(s),r=this.tips.filter(c=>c.group===t).map(c=>c.id),n=this.presets.filter(c=>!i.has(c.id)),a=new Set(n.flatMap(c=>c.tipId?[c.tipId]:[])),o=r.filter(c=>!a.has(c)),{patterns:h}=await this._store.deleteMany({presets:s,tips:o,unusedPatterns:this._unusedPatterns(n)});this.presets=this.presets.filter(c=>!i.has(c.id));for(const c of o)this._forgetTip(c);this._forgetPatterns(h),this._changes++,this._onChange(),this._onStored()}_unusedPatterns(t){const s=new Set(t.map(i=>i.descriptor?.texture?.pattern));return[...this._knownPatterns].filter(i=>!s.has(i))}_forgetPatterns(t){for(const s of t)Js(s,this),this._knownPatterns.delete(s)}async _prunePatterns(){const t=this._unusedPatterns(this.presets);if(t.length!==0)try{this._forgetPatterns((await this._store.deleteMany({unusedPatterns:t})).patterns)}catch(s){console.error("Could not remove unused brush patterns:",s)}}async removePreset(t){const s=this.presets.find(i=>i.id===t)?.tipId;await this._store.deletePreset(t),this.presets=this.presets.filter(i=>i.id!==t),s&&!this.presets.some(i=>i.tipId===s)&&this._forgetTip(s),this._changes++,await this._prunePatterns(),this._changes++,this._onChange(),this._onStored()}async removeTip(t){await this._store.deleteTip(t),this.presets=this.presets.filter(s=>s.tipId!==t),this._forgetTip(t),this._changes++,await this._prunePatterns(),this._changes++,this._onChange(),this._onStored()}_forgetTip(t){this.tips=this.tips.filter(s=>s.id!==t),this._known.delete(t),this._deleted.set(t,(this._deleted.get(t)??0)+1),Rr(t,this)}}class B extends Error{constructor(t,s=!1){super(t),this.fatal=s,this.name="AbrParseError"}}const Y={maxSide:5e3,maxPixels:5e3*5e3,maxBrushes:2e3,maxPatterns:500,maxDecodedBytes:1.5*1024*1024*1024,maxDescriptorDepth:24,maxDescriptorNodes:4e5,maxStringLength:4096};function Rt(e){const t=e.u32();if(t>Y.maxStringLength)throw new B("The file holds a name that is too long.");return String.fromCharCode(...e.bytes(t===0?4:t))}function ar(e,t=1){if(e.nodes+=t,e.nodes>Y.maxDescriptorNodes)throw new B("The brush settings in this file are too large.")}function ua(e,t,s){if(t>Y.maxDescriptorDepth)throw new B("The brush settings in this file are nested too deeply.");const i=e.unicode(Y.maxStringLength),r=Rt(e),n=new Map,a=e.u32();if(a>e.remaining/8)throw new B("The brush settings in this file are damaged.");ar(s,a);for(let o=0;o<a;o++){const h=Rt(e);n.set(h,Es(e,e.fourCC(),t,s))}return{kind:"object",classId:r,name:i,items:n}}function Es(e,t,s,i){switch(ar(i),t){case"Objc":case"GlbO":return ua(e,s+1,i);case"VlLs":{if(s+1>Y.maxDescriptorDepth)throw new B("The brush settings in this file are nested too deeply.");const r=e.u32();if(r>e.remaining/4)throw new B("The brush settings in this file are damaged.");const n=[];for(let a=0;a<r;a++)n.push(Es(e,e.fourCC(),s+1,i));return n}case"doub":return e.f64();case"UntF":return{kind:"unit",unit:e.fourCC(),value:e.f64()};case"TEXT":return e.unicode(Y.maxStringLength);case"enum":return{kind:"enum",enumType:Rt(e),value:Rt(e)};case"long":return e.i32();case"comp":return e.i64();case"bool":return e.u8()!==0;case"type":case"GlbC":return e.unicode(Y.maxStringLength),Rt(e);case"alis":case"tdta":{const r=e.u32();return e.bytes(r)}case"obj ":return uh(e,i);default:throw new B(`The brush settings use a value type this reader doesn't know ("${t.replace(/[^\x20-\x7e]/g,"?")}").`)}}function uh(e,t){const s=e.u32();if(s>e.remaining/4)throw new B("The brush settings in this file are damaged.");const i=[];for(let r=0;r<s;r++){const n=e.fourCC();switch(ar(t),n){case"prop":e.unicode(Y.maxStringLength),Rt(e),i.push(Rt(e));break;case"Clss":e.unicode(Y.maxStringLength),i.push(Rt(e));break;case"Enmr":e.unicode(Y.maxStringLength),Rt(e),Rt(e),i.push(Rt(e));break;case"rele":e.unicode(Y.maxStringLength),Rt(e),i.push(e.i32());break;case"Idnt":case"indx":i.push(e.i32());break;case"name":e.unicode(Y.maxStringLength),Rt(e),i.push(e.unicode(Y.maxStringLength));break;default:throw new B("The brush settings hold a reference this reader cannot follow.")}}return i}function gt(e,t){const s=e?.items.get(t);return s&&typeof s=="object"&&!Array.isArray(s)&&!(s instanceof Uint8Array)&&s.kind==="object"?s:void 0}function at(e,t){const s=e?.items.get(t);return typeof s=="boolean"?s:void 0}function ks(e,t){const s=e?.items.get(t);return typeof s=="string"?s:void 0}function J(e,t){const s=e?.items.get(t);if(typeof s=="number")return Number.isFinite(s)?s:void 0;if(s&&typeof s=="object"&&!Array.isArray(s)&&s.kind==="unit"){const i=s.value;return Number.isFinite(i)?i:void 0}}function ei(e,t){const s=e?.items.get(t);return s&&typeof s=="object"&&!Array.isArray(s)&&s.kind==="enum"?s.value:void 0}class Us{constructor(t,s=0,i){const r=t instanceof ArrayBuffer?new Uint8Array(t):new Uint8Array(t.buffer,t.byteOffset,t.byteLength);this._bytes=r,this._view=new DataView(r.buffer,r.byteOffset,r.byteLength),this.end=Math.min(i??r.length,r.length),this.pos=s}get remaining(){return this.end-this.pos}slice(t){this.need(t);const s=new Us(this._bytes,this.pos,this.pos+t);return this.pos+=t,s}need(t){if(!Number.isInteger(t)||t<0||this.pos+t>this.end)throw new B("The file ends unexpectedly (it may be damaged or cut short).")}skip(t){this.need(t),this.pos+=t}seek(t){if(!Number.isInteger(t)||t<0||t>this.end)throw new B("A record points outside the file.");this.pos=t}u8(){return this.need(1),this._view.getUint8(this.pos++)}i16(){this.need(2);const t=this._view.getInt16(this.pos);return this.pos+=2,t}u16(){this.need(2);const t=this._view.getUint16(this.pos);return this.pos+=2,t}i32(){this.need(4);const t=this._view.getInt32(this.pos);return this.pos+=4,t}u32(){this.need(4);const t=this._view.getUint32(this.pos);return this.pos+=4,t}f64(){this.need(8);const t=this._view.getFloat64(this.pos);return this.pos+=8,t}i64(){this.need(8);const t=Number(this._view.getBigInt64(this.pos));return this.pos+=8,t}bytes(t){this.need(t);const s=this._bytes.subarray(this.pos,this.pos+t);return this.pos+=t,s}fourCC(){return String.fromCharCode(...this.bytes(4))}peekFourCC(){return this.remaining<4?"":String.fromCharCode(...this._bytes.subarray(this.pos,this.pos+4))}pascal(){const t=this.u8();return String.fromCharCode(...this.bytes(t))}unicode(t){const s=this.u32();if(s>t)throw new B("The file holds a text that is too long.");this.need(s*2);let i="";for(let r=0;r<s;r++)i+=String.fromCharCode(this.u16());return i.replace(/\0+$/,"")}}function ph(e,t,s,i){const r=[];for(let n=0;n<t;n++)r.push(e.u16());for(let n=0;n<t;n++){const a=e.bytes(r[n]);fh(a,i.subarray(n*s,(n+1)*s))}}function fh(e,t){let s=0,i=0;for(;s<e.length&&i<t.length;){const r=e[s++]<<24>>24;if(r>=0){const n=Math.min(r+1,t.length-i,e.length-s);t.set(e.subarray(s,s+n),i),s+=r+1,i+=n}else if(r!==-128){if(s>=e.length)break;const n=Math.min(1-r,t.length-i);t.fill(e[s++],i,i+n),i+=n}}}function _h(e,t){if(e.bytes+=t,e.bytes>Y.maxDecodedBytes)throw new B("This brush file is too large to import.",!0)}function pa(e,t,s){if(!(e>=1&&t>=1)||e>Y.maxSide||t>Y.maxSide||e*t>Y.maxPixels)throw new B(`A ${s} has an unusable size (${e}×${t}).`)}function or(e,t,s,i,r,n){if(pa(t,s,"brush image"),i!==8&&i!==16)throw new B(`A brush image has a pixel depth of ${i} bits, which isn't supported.`);if(r!==0&&r!==1)throw new B("A brush image uses an unknown compression.");const a=i/8,o=t*a;_h(n,t*s*(a+1));let h;if(r===0?h=e.bytes(o*s):(e.need(s*2),h=new Uint8Array(o*s),ph(e,s,o,h)),i===8)return h;const c=new Uint8Array(t*s);for(let l=0,d=0;l<c.length;l++,d+=2)c[l]=Math.round((h[d]<<8|h[d+1])*255/65535);return c}const hr=e=>Math.ceil(e/4)*4;function*mh(e){const t=new Us(e);if(t.remaining<4)throw new B("This is not a Photoshop brush file (.abr): it is too short.");const s=t.i16(),i={bytes:0};if(s===1||s===2)return yield*gh(t,s,i),{version:s,subversion:0};if(s===6||s===7||s===9||s===10){const r=t.i16();if(r!==1&&r!==2)throw new B(`This brush file has a layout (${s}.${r}) this reader doesn't know.`);return yield*bh(t,r,i),{version:s,subversion:r}}throw new B(s>10||s<1?"This is not a Photoshop brush file (.abr), or it is from a version this reader does not know.":`Photoshop brush files of version ${s} are not supported.`)}function*gh(e,t,s){const i=e.i16();if(i<0)throw new B("This brush file is damaged.");const r=e.end;let n=0;for(let a=0;a<i;a++){if(e.remaining<6){yield{type:"skip",what:"preset",reason:"The file ends before its last brushes."};return}if(a>=Y.maxBrushes){yield{type:"skip",what:"preset",reason:`Only the first ${Y.maxBrushes} brushes are read.`};return}const o=e.u16(),h=e.u32();if(h>e.remaining){yield{type:"skip",what:"preset",reason:"The file ends in the middle of a brush."};return}const c=e.slice(h);try{if(o===1)yield{type:"preset",preset:yh(c,a)};else if(o===2){const{sample:l,preset:d}=vh(c,t,a,s);yield{type:"sample",sample:l},yield{type:"preset",preset:d}}else yield{type:"skip",what:"preset",reason:`A brush of unknown type ${o} was left out.`}}catch(l){if(!(l instanceof B)||l.fatal)throw l;yield{type:"skip",what:"preset",reason:l.message}}n=e.pos,yield{type:"progress",done:n,total:r}}}function yh(e,t){if(e.remaining<14)throw new B("A computed brush is too short.");e.skip(4);const s=e.u16(),i=e.u16(),r=e.u16(),n=e.i16(),a=e.u16();if(s<1||s>999||i<1||i>2500||r>100||a>100||n<-360||n>360)throw new B("A computed brush has values out of range and was left out.");return{name:`Brush ${t+1}`,tip:{kind:"computed",diameter:i,hardness:a/100,roundness:r/100,angle:n,spacing:s}}}function vh(e,t,s,i){e.skip(4);const r=e.u16(),n=t===2?e.unicode(Y.maxStringLength).trim():"";e.skip(1),e.skip(8);const a=e.i32(),o=e.i32(),h=e.i32(),c=e.i32(),l=e.i16(),d=e.u8(),u=c-o,_=h-a,f=or(e,u,_,l,d,i),m=`legacy-${s}`,p=n||`Brush ${s+1}`;return{sample:{id:m,name:p,width:u,height:_,alpha:f},preset:{name:p,tip:{kind:"sampled",sampleId:m,diameter:Math.max(u,_),roundness:1,angle:0,spacing:r,flipX:!1,flipY:!1}}}}function*bh(e,t,s){const i=e.end,r={samples:0,patterns:0,presets:0,descriptorNodes:{nodes:0}};for(;e.remaining>=12;){if(e.fourCC()!=="8BIM"){yield{type:"skip",what:"preset",reason:"Unrecognised data after the brush sections was ignored."};return}const n=e.fourCC(),a=e.u32(),o=a>e.remaining,h=e.slice(Math.min(a,e.remaining));if(o&&(yield{type:"skip",what:"preset",reason:"The file ends in the middle of a section."}),n==="samp"?yield*wh(h,t,s,r,i):n==="patt"?yield*Mh(h,s,r):n==="desc"&&(yield*xh(h,r)),o)return;a%4!==0&&e.peekFourCC()!=="8BIM"&&e.skip(Math.min(hr(a)-a,e.remaining)),yield{type:"progress",done:e.pos,total:i}}}function*wh(e,t,s,i,r){for(;e.remaining>=4;){if(i.samples>=Y.maxBrushes){yield{type:"skip",what:"sample",reason:`Only the first ${Y.maxBrushes} brush images are read.`};return}i.samples++;const n=hr(e.u32()),a=e.slice(Math.min(n,e.remaining));try{const o=a.pascal();if(!o)throw new B("A brush image has no id.");a.skip(t===1?10:264);const h=a.i32(),c=a.i32(),l=a.i32(),d=a.i32(),u=a.i16(),_=a.u8(),f=d-c,m=l-h,p=or(a,f,m,u,_,s);yield{type:"sample",sample:{id:o,width:f,height:m,alpha:p}}}catch(o){if(!(o instanceof B)||o.fatal)throw o;yield{type:"skip",what:"sample",reason:o.message}}yield{type:"progress",done:e.pos,total:r}}}function*xh(e,t){const s=t.descriptorNodes;try{if(e.u32()!==16)throw new B("The brush settings are in a format this reader does not know.");e.unicode(Y.maxStringLength),Dr(e);const r=e.u32();if(r>e.remaining/8)throw new B("The brush settings are damaged.");for(let n=0;n<r;n++){const a=Dr(e),o=e.fourCC();if(a!=="Brsh"||o!=="VlLs"){Es(e,o,1,s);continue}const h=e.u32();if(h>e.remaining/4)throw new B("The list of brushes is damaged.");for(let c=0;c<h;c++){const l=e.fourCC();if(l!=="Objc"){Es(e,l,2,s);continue}if(t.presets>=Y.maxBrushes){yield{type:"skip",what:"preset",reason:`Only the first ${Y.maxBrushes} brushes are read.`};return}const d=ua(e,2,s),u=kh(d,t.presets++);u?yield{type:"preset",preset:u}:yield{type:"skip",what:"preset",reason:"A brush has no tip this reader can use."}}}}catch(i){if(!(i instanceof B)||i.fatal)throw i;yield{type:"skip",what:"preset",reason:i.message}}}function Dr(e){const t=e.u32();if(t>Y.maxStringLength)throw new B("The file holds a name that is too long.");return String.fromCharCode(...e.bytes(t===0?4:t))}function kh(e,t){const s=gt(e,"Brsh");if(!s)return null;const i=(ks(e,"Nm  ")??ks(s,"Nm  ")??"").trim()||`Brush ${t+1}`,r={diameter:J(s,"Dmtr")??0,roundness:(J(s,"Rndn")??100)/100,angle:J(s,"Angl")??0,spacing:J(s,"Spcn")??25};let n;if(s.classId==="sampledBrush"){const a=ks(s,"sampledData");if(!a)return null;n={kind:"sampled",sampleId:a,...r,flipX:at(s,"flipX")===!0,flipY:at(s,"flipY")===!0}}else s.classId==="computedBrush"?n={kind:"computed",...r,hardness:(J(s,"Hrdn")??100)/100}:n={kind:"other",className:s.classId};return{name:i,tip:n,settings:e}}function*Mh(e,t,s){for(;e.remaining>=4;){if(s.patterns>=Y.maxPatterns){yield{type:"skip",what:"pattern",reason:`Only the first ${Y.maxPatterns} patterns are read.`};return}s.patterns++;const i=e.u32();if(i>e.remaining){yield{type:"skip",what:"pattern",reason:"The file ends in the middle of a pattern."};return}const r=e.slice(i),n=Math.min(hr(i)-i,e.remaining);e.skip(n);try{yield{type:"pattern",pattern:Ch(r,t)}}catch(a){if(!(a instanceof B)||a.fatal)throw a;yield{type:"skip",what:"pattern",reason:a.message}}}}function Ch(e,t){if(e.u32()!==1)throw new B("A pattern is in a format this reader does not know.");const s=e.u32();if(s!==1&&s!==2&&s!==3)throw new B("A pattern is not grayscale, indexed or RGB, and was left out.");e.skip(4);const i=e.unicode(Y.maxStringLength).trim(),r=e.pascal();if(!r)throw new B("A pattern has no id.");let n=null;if(s===2&&(n=e.bytes(768).slice(),e.skip(4)),e.u32()!==3)throw new B("A pattern is in a format this reader does not know.");const a=e.u32(),o=Math.min(e.end,e.pos+a),h=e.i32(),c=e.i32(),l=e.i32(),u=e.i32()-c,_=l-h;pa(u,_,"pattern");const f=e.u32(),m=s===3?3:1,p=[];for(let y=0;y<Math.min(f,56)+2&&p.length<m&&e.pos+4<=o;y++){if(e.u32()===0)continue;const w=e.u32();if(w>o-e.pos)throw new B("A pattern is cut short.");const x=e.pos+w,b=new Us(e.bytes(w));b.skip(4);const S=b.i32(),E=b.i32(),D=b.i32();if(b.i32()-E!==u||D-S!==_)throw new B("A pattern has a channel of another size than the pattern.");const I=b.u16(),k=b.u8();p.push(or(b,u,_,I,k,t)),e.seek(x)}if(p.length<m)throw new B("A pattern is missing its pixels.");const g=new Uint8Array(u*_);if(s===3)for(let y=0;y<g.length;y++)g[y]=Math.round(.2126*p[0][y]+.7152*p[1][y]+.0722*p[2][y]);else if(s===2&&n)for(let y=0;y<g.length;y++){const w=p[0][y]*3;g[y]=Math.round(.2126*n[w]+.7152*n[w+1]+.0722*n[w+2])}else g.set(p[0]);return{id:r,name:i||r,width:u,height:_,data:g}}const Ar=150,Sh=.25,Ph=.05,Th=1,Lr=256*1024*1024,$h=128*1024*1024,Ih=1024,Eh=80,Dt=(e,t,s)=>Math.max(t,Math.min(s,e)),Ot=(e,t)=>Dt((J(e,t)??0)/100,0,1);function fa(e,t){return e.replace(/[\u0000-\u001f\u007f]/g," ").replace(/\s+/g," ").trim().slice(0,Eh)||t}function Rh(e,t,s){if(!t&&!s)return e;const{width:i,height:r,alpha:n}=e,a=new Uint8Array(n.length);for(let o=0;o<r;o++){const h=s?r-1-o:o;for(let c=0;c<i;c++)a[o*i+c]=n[h*i+(t?i-1-c:c)]}return{width:i,height:r,alpha:a}}function Dh(e){const t=aa({width:e.width,height:e.height,alpha:e.alpha});if(!t)return null;const s=Math.max(e.width,e.height),i=Ys(t,na);return{mask:{width:i.width,height:i.height,alpha:i.alpha.slice()},cropShare:Math.max(t.width,t.height)/s,originalLong:s}}function Ah(e,t){let s=2166136261;for(const a of[t.width,t.height,...t.alpha])s=Math.imul(s^a,16777619);const i=(s>>>0).toString(16).padStart(8,"0"),r=e.id.replace(/[^A-Za-z0-9-]/g,a=>`_${a.charCodeAt(0).toString(16)}_`),n=tr-4-1-i.length;return`abr-${r.length<=n?r:`long-${r.length}`}-${i}`}function Lh(e){const t=Ys({width:e.width,height:e.height,alpha:e.data},Ih);return{tile:{id:Ah(e,t),name:fa(e.name,"Pattern"),width:t.width,height:t.height,data:t.alpha.slice()},shrink:Math.max(e.width,e.height)/Math.max(t.width,t.height)}}const We=2,jh=3,zh=1;function Oh(e){switch(e){case jh:return"tilt";case 5:case 6:return"direction";case 7:case 8:return"twist";default:return"fixed"}}const jr={Nrml:"normal",Mltp:"multiply",Scrn:"screen",Ovrl:"overlay",Drkn:"darken",Lghn:"lighten",CDdg:"color-dodge",CBrn:"color-burn","H   ":"hue",Strt:"saturation","Clr ":"color",Lmns:"luminosity",Bhnd:"behind",Clar:"clear"};function zr(e){return e?e==="Sbtr"?"subtract":/height|hght|lnrh/i.test(e)?"height":"multiply":"multiply"}const Bh=new Set(["Nm  ","Brsh","useTipDynamics","flipX","flipY","brushProjection","minimumDiameter","minimumRoundness","tiltScale","szVr","angleDynamics","roundnessDynamics","useScatter","Spcn","Cnt ","bothAxes","countDynamics","scatterDynamics","dualBrush","brushGroup","useTexture","TxtC","interpretation","textureBlendMode","textureDepth","minimumDepth","textureDepthDynamics","Txtr","textureScale","InvT","protectTexture","textureBrightness","textureContrast","usePaintDynamics","prVr","opVr","wtVr","mxVr","useColorDynamics","clVr","H   ","Strt","Brgh","purity","colorDynamicsPerTip","Wtdg","Nose","Rpt ","useBrushSize","useBrushPose","overridePoseAngle","overridePoseTiltX","overridePoseTiltY","overridePosePressure","brushPosePressure","brushPoseTiltX","brushPoseTiltY","brushPoseAngle","toolOptions"]);function Hh(e,t,s,i){for(const o of t.items.keys())Bh.has(o)||s.unmapped.add(`preset:${o}`);const r=o=>s.unmapped.add(o);if(at(t,"useTipDynamics")){const o=gt(t,"szVr"),h=J(o,"bVTy");e.jitter.size=Ot(o,"jitter"),e.pressureSize=h===We,e.dynamics.minSize=Ot(t,"minimumDiameter"),h===zh?(e.fade.sizeLength=Dt(Math.max(0,J(o,"fStp")??0),0,ea),e.fade.sizeUnit="dabs"):h===4&&r("szVr:stylusWheel");const c=gt(t,"angleDynamics");e.jitter.angle=Ot(c,"jitter"),e.tip.orientation=Oh(J(c,"bVTy")),e.jitter.roundness=Ot(gt(t,"roundnessDynamics"),"jitter"),J(t,"minimumRoundness")&&r("minimumRoundness"),(at(t,"flipX")||at(t,"flipY"))&&r("flipJitter"),at(t,"brushProjection")&&r("brushProjection")}else e.pressureSize=!1;if(at(t,"useScatter")&&(e.scatter.amount=Dt((J(gt(t,"scatterDynamics"),"jitter")??0)/100/i,0,er),e.scatter.bothAxes=at(t,"bothAxes")===!0,e.scatter.count=Math.round(Dt(J(t,"Cnt ")??1,1,sr)),e.scatter.countJitter=Ot(gt(t,"countDynamics"),"jitter")),at(t,"useTexture")){const o=gt(t,"Txtr"),h=ks(o,"Idnt"),c=h?s.patterns.get(h.toLowerCase()):void 0;if(c){s.usedPatterns.add(c.tile.id);const l=J(gt(t,"textureDepthDynamics"),"bVTy");e.texture={pattern:c.tile.id,scale:Dt((J(t,"textureScale")??100)/100*c.shrink,sa,ia),depth:Ot(t,"textureDepth"),mode:zr(ei(t,"textureBlendMode")),invert:at(t,"InvT")===!0,depthPressure:l===We,perTip:at(t,"TxtC")===!0};const d=ei(t,"textureBlendMode");d&&d!=="Mltp"&&d!=="Sbtr"&&zr(d)==="multiply"&&r(`textureBlendMode:${d}`),(J(t,"textureBrightness")||J(t,"textureContrast"))&&r("textureBrightnessContrast")}else r("texture:patternMissing")}if(at(t,"useColorDynamics")){const o=gt(t,"clVr");e.color={hue:Ot(t,"H   "),saturation:Ot(t,"Strt"),brightness:Ot(t,"Brgh"),purity:Dt((J(t,"purity")??0)/100,-1,1),fgBg:Ot(o,"jitter"),fgBgPressure:J(o,"bVTy")===We,perTip:at(t,"colorDynamicsPerTip")===!0}}if(at(t,"usePaintDynamics")){const o=gt(t,"prVr"),h=gt(t,"opVr");e.jitter.flow=Math.max(Ot(o,"jitter"),Ot(h,"jitter")),e.pressureOpacity=J(h,"bVTy")===We||J(o,"bVTy")===We,(J(gt(t,"wtVr"),"jitter")||J(gt(t,"mxVr"),"jitter"))&&r("wetnessMixJitter")}at(t,"Rpt ")&&(e.airbrush=!0),at(t,"Wtdg")&&r("wetEdges"),at(t,"Nose")&&r("noise"),at(t,"useBrushPose")&&r("brushPose");const n=gt(t,"dualBrush");n&&at(n,"useDualBrush")&&r("dualBrush");const a=gt(t,"toolOptions");if(a){const o=J(a,"Opct"),h=J(a,"flow");o!==void 0&&(e.opacity=Dt(o/100,0,1)),h!==void 0&&(e.flow=Dt(h/100,0,1));const c=ei(a,"Md  ");c&&(jr[c]?e.blendMode=jr[c]:r(`mode:${c}`))}}function Fh(e,t,s){const i=t.tip;if(i.kind==="other")return;e.spacing=Dt(i.spacing/100/(i.kind==="sampled"&&s?s.cropShare:1),Ph,Th);const r=(-i.angle%360+360)%360;if(i.kind==="sampled"){e.tip={...Q("image"),angle:r};const n=i.diameter>0?i.diameter:s.originalLong;e.size=Dt(Math.round(n*s.cropShare),1,Ar);return}e.size=Dt(Math.round(i.diameter),1,Ar),e.hardness=Dt(i.hardness,0,1),i.roundness<.97?e.tip={...Q("calligraphy"),aspect:Dt(1/Math.max(i.roundness,.05),1,10),angle:r,orientation:"fixed"}:e.tip=Q("round")}function Nh(e){const t=e.sliceMs??12,s=e.yieldToUi??(()=>new Promise(r=>setTimeout(r,0)));let i=Date.now();return async()=>{Date.now()-i<t||(await s(),i=Date.now())}}async function Xh(e,t={}){const s=Nh(t),i=new Map,r=new Map,n=[],a={tips:[],brushes:[],patterns:[],skipped:0,skipReasons:[],unmapped:[]},o=_=>{a.skipped++,a.skipReasons.length<20&&!a.skipReasons.includes(_)&&a.skipReasons.push(_)};let h=0,c=0;const l=mh(e);for(;;){const _=l.next();if(_.done)break;const f=_.value;if(f.type==="sample"){const m=Dh(f.sample);m&&h+m.mask.alpha.length>Lr?(o("Some brush images were left out to stay within memory limits."),i.set(f.sample.id,null)):(m&&(h+=m.mask.alpha.length),i.set(f.sample.id,m))}else if(f.type==="pattern"){const m=Lh(f.pattern);c+m.tile.data.length>$h?o("Some patterns were left out to stay within memory limits."):(c+=m.tile.data.length,r.set(f.pattern.id.toLowerCase(),m))}else f.type==="preset"?n.push(f.preset):f.type==="skip"?o(f.reason):t.onProgress?.(f.total>0?.6*Math.min(1,f.done/f.total):0);await s()}if(n.length===0){let _=0;for(const[f,m]of i)m&&n.push({name:`Brush ${++_}`,tip:{kind:"sampled",sampleId:f,diameter:m.originalLong,roundness:1,angle:0,spacing:Sh*100,flipX:!1,flipY:!1}})}const d={patterns:r,usedPatterns:new Set,unmapped:new Set},u=new Map;for(let _=0;_<n.length;_++){const f=n[_],m=f.tip;let p=null;if(m.kind==="other"){o("Bristle and erodible tips are not supported.");continue}if(m.kind==="sampled"){if(!i.has(m.sampleId)){o("A brush names a tip image that is not in the file.");continue}if(p=i.get(m.sampleId),!p){o("A brush tip paints nothing (it is blank) or was too large to keep.");continue}}const g=Ws({pressureSize:!1,pressureOpacity:!1});Fh(g,f,p),f.settings&&Hh(g,f.settings,d,p?p.cropShare:1),m.kind==="sampled"&&m.roundness<.97&&d.unmapped.add("sampledBrush:roundness");const y={name:fa(f.name,`Brush ${_+1}`),descriptor:g};if(m.kind==="sampled"&&p){const w=`${m.sampleId}|${m.flipX?1:0}${m.flipY?1:0}`;let x=u.get(w);if(!x){const b=m.flipX||m.flipY;if(b&&h+p.mask.alpha.length>Lr){o("Some brushes were left out to stay within memory limits.");continue}b&&(h+=p.mask.alpha.length),x={key:w,name:y.name,mask:Rh(p.mask,m.flipX,m.flipY),spacing:g.spacing},u.set(w,x),a.tips.push(x)}y.tipKey=w}a.brushes.push(y),t.onProgress?.(.6+.4*((_+1)/n.length)),await s()}for(const{tile:_}of r.values())d.usedPatterns.has(_.id)&&a.patterns.push(_);if(a.unmapped=[...d.unmapped].sort(),a.brushes.length===0)throw new B(a.skipped>0?"None of the brushes in this file could be imported.":"This file contains no brushes.");return a}function Wh(e,t){t.length}const Yh=2048;function _a(e){return th(e.data,e.width,e.height)}async function Uh(e){const t=await createImageBitmap(e);try{const s=Math.min(1,Yh/Math.max(t.width,t.height)),i=Math.max(1,Math.round(t.width*s)),r=Math.max(1,Math.round(t.height*s)),n=document.createElement("canvas");n.width=i,n.height=r;const a=n.getContext("2d");return a.imageSmoothingQuality="high",a.drawImage(t,0,0,i,r),_a(a.getImageData(0,0,i,r))}finally{t.close?.()}}class At extends Error{constructor(t,s){super(t),this.cause=s,this.name="StorageError"}}class Ne extends At{constructor(){super(...arguments),this.name="StorageNotFoundError"}}class ae extends At{constructor(){super(...arguments),this.name="StorageQuotaError"}}class Vh extends At{constructor(){super(...arguments),this.name="StorageConflictError"}}class Le extends At{constructor(t,s=!1,i){super(t,i),this.versionChange=s,this.name="StorageClosedError"}}const Gh=20;class qh{constructor(t,s){this._storage=t,this._maxStamps=s?.maxStampsPerProject??Gh}get storage(){return this._storage}async deleteProject(t){const s=new Set,[i,r,n,a]=await Promise.all([this._storage.state.get(t).catch(()=>null),this._storage.history.getEntries(t).catch(()=>[]),this._storage.stamps.list(t).catch(()=>[]),this._storage.projects.get(t).catch(()=>null)]);a?.thumbnailRef&&s.add(a.thumbnailRef),i?.layers.forEach(o=>s.add(o.imageBlobRef));for(const o of r)xe(o?.entry,s);n.forEach(o=>s.add(o.blobRef)),await this._storage.projects.delete(t),await Promise.all([this._storage.state.delete(t),this._storage.history.deleteForProject(t),this._storage.stamps.deleteForProject(t)]).catch(o=>console.error("Cascade delete partial failure:",o)),s.size>0&&this._storage.blobs.deleteMany([...s]).catch(()=>{})}async addStamp(t,s){const i=await this._storage.stamps.add(t,s),r=await this._storage.stamps.list(t);if(r.length>this._maxStamps){const n=r.filter(a=>a.id!==i.id).sort((a,o)=>a.createdAt-o.createdAt).slice(0,r.length-this._maxStamps);for(const a of n)await this._storage.stamps.delete(a.id)}return i}async collectGarbage(){if(!this._storage.blobs.gc)return 0;const t=await this._storage.projects.list(),s=new Set;for(const i of t){const[r,n,a]=await Promise.all([this._storage.state.get(i.id),this._storage.history.getEntries(i.id),this._storage.stamps.list(i.id)]);i.thumbnailRef&&s.add(i.thumbnailRef),r?.layers.forEach(o=>s.add(o.imageBlobRef)),n.forEach(o=>xe(o?.entry,s)),a.forEach(o=>s.add(o.blobRef)),await new Promise(o=>setTimeout(o,0))}return this._storage.blobs.gc(s)}}function xe(e,t){if(!si(e))return;const s=e,i=n=>{si(n)&&typeof n.blobRef=="string"&&t.add(n.blobRef)},r=n=>{si(n)&&i(n.imageData)};switch(s.type){case"draw":case"patch":case"transform":i(s.before),i(s.after);break;case"add-layer":case"delete-layer":r(s.layer);break;case"crop":case"merge":Array.isArray(s.beforeLayers)&&s.beforeLayers.forEach(r),Array.isArray(s.afterLayers)&&s.afterLayers.forEach(r);break}}function si(e){return typeof e=="object"&&e!==null}const ma="storage-backend",ga="project-service";class Kh{constructor(t){this._newId=t,this._blobs=new Map}async put(t){const s=this._newId();return this._blobs.set(s,t instanceof Blob?t:new Blob([t])),s}async get(t){const s=this._blobs.get(t);if(!s)throw new Ne(`Blob ${t} not found`);return s}async delete(t){this._blobs.delete(t)}async deleteMany(t){for(const s of t)this._blobs.delete(s)}async gc(t){let s=0;for(const i of[...this._blobs.keys()])t.has(i)||(this._blobs.delete(i),s++);return s}}class Zh{constructor(t){this._newId=t,this._projects=new Map}async list(t){const s=Array.from(this._projects.values()),i=t?.orderBy??"updatedAt",r=t?.direction==="asc"?1:-1;return s.sort((n,a)=>r*(n[i]-a[i])),s}async get(t){return this._projects.get(t)??null}async create(t){const s=Date.now(),i={id:this._newId(),name:t.name,createdAt:s,updatedAt:s,thumbnailRef:t.thumbnailRef??null};return this._projects.set(i.id,i),i}async update(t,s){const i=this._projects.get(t);if(!i)throw new Ne(`Project ${t}`);return s.name!==void 0&&(i.name=s.name),s.thumbnailRef!==void 0&&(i.thumbnailRef=s.thumbnailRef),i.updatedAt=Date.now(),i}async delete(t){this._projects.delete(t)}}class Qh{constructor(){this._states=new Map}async get(t){return this._states.get(t)??null}async save(t){this._states.set(t.projectId,t)}async delete(t){this._states.delete(t)}}class Jh{constructor(){this._entries=new Map}async getEntries(t){return[...this._entries.get(t)??[]].sort((s,i)=>s.index-i.index)}async putEntries(t,s){const i=this._entries.get(t)??[];this._entries.set(t,[...i,...s])}async replaceAll(t,s){this._entries.set(t,[...s])}async updateEntries(t,s,i){const r=new Set(s),n=(this._entries.get(t)??[]).filter(a=>!r.has(a.index));this._entries.set(t,[...n,...i])}async deleteForProject(t){this._entries.delete(t)}}class tc{constructor(t,s){this._blobs=t,this._newId=s,this._stamps=new Map}async list(t){return Array.from(this._stamps.values()).filter(s=>s.projectId===t).sort((s,i)=>i.createdAt-s.createdAt)}async add(t,s,i=Date.now()){const r=await this._blobs.put(s),n={id:this._newId(),projectId:t,blobRef:r,createdAt:i};return this._stamps.set(n.id,n),n}async delete(t){const s=this._stamps.get(t);s&&(this._stamps.delete(t),this._blobs.delete(s.blobRef).catch(()=>{}))}async deleteForProject(t){const s=[];for(const[i,r]of this._stamps)r.projectId===t&&(s.push(r.blobRef),this._stamps.delete(i));s.length>0&&this._blobs.deleteMany(s).catch(()=>{})}}class ec{constructor(){this._tips=new Map,this._presets=new Map,this._patterns=new Map}async list(){return{tips:[...this._tips.values()].map(t=>structuredClone(t.meta)).sort((t,s)=>t.createdAt-s.createdAt),presets:[...this._presets.values()].map(t=>structuredClone(t)).sort((t,s)=>t.createdAt-s.createdAt)}}async getMask(t){const s=this._tips.get(t);return s?{width:s.mask.width,height:s.mask.height,alpha:s.mask.alpha.slice()}:null}async putBatch(t){for(const s of t.patterns??[])this._patterns.set(s.id,{...s,data:s.data.slice()});for(const{meta:s,mask:i}of t.tips??[])this._tips.set(s.id,{meta:structuredClone(s),mask:{width:i.width,height:i.height,alpha:i.alpha.slice()}});for(const s of t.presets??[])this._presets.set(s.id,structuredClone(s))}async listPatterns(){return[...this._patterns.values()].map(t=>({...t,data:t.data.slice()})).sort((t,s)=>t.createdAt-s.createdAt)}async deleteMany(t){for(const r of t.presets??[])this._presets.delete(r);for(const r of t.tips??[])this._tips.delete(r);const s=new Set([...this._presets.values()].map(r=>r.descriptor?.texture?.pattern)),i=(t.unusedPatterns??[]).filter(r=>!s.has(r)&&this._patterns.has(r));for(const r of i)this._patterns.delete(r);return{patterns:i}}async deletePreset(t){const s=this._presets.get(t)?.tipId;this._presets.delete(t),s&&![...this._presets.values()].some(i=>i.tipId===s)&&this._tips.delete(s)}async deleteTip(t){this._tips.delete(t);for(const[s,i]of this._presets)i.tipId===t&&this._presets.delete(s)}}class sc{constructor(t=jt){this.brushes=new ec,this.blobs=new Kh(t),this.projects=new Zh(t),this.state=new Qh,this.history=new Jh,this.stamps=new tc(this.blobs,t)}async init(){}async dispose(){}}function U(e,t,s){try{return e.transaction(t,s)}catch(i){throw i instanceof DOMException&&i.name==="InvalidStateError"?new Le("Storage is closed",!1,i):ht(i)}}function ht(e){if(e instanceof DOMException)switch(e.name){case"QuotaExceededError":return new ae(e.message,e);case"NotFoundError":return new Ne(e.message,e);case"ConstraintError":return new Vh(e.message,e);default:return new At(e.message,e)}return e instanceof Error?new At(e.message,e):new At(String(e))}function Z(e){return ht(e.error??new DOMException("The transaction was aborted","AbortError"))}function mt(e,t){return ht(e.target?.error??t.error)}function ic(e,t,s){const i=e.createObjectStore("blobs");if(s<1)return;function r(f){if(f?.blob){const m=jt();return i.put(f.blob,m),f.blobRef=m,delete f.blob,!0}return!1}function n(f){return r(f?.imageData)}const o=t.objectStore("projects").openCursor();o.onsuccess=function(){const f=o.result;if(!f)return;const m=f.value;if(m.thumbnail){const p=jt();i.put(m.thumbnail,p),m.thumbnailRef=p,delete m.thumbnail,f.update(m)}f.continue()};const c=t.objectStore("project-state").openCursor();c.onsuccess=function(){const f=c.result;if(!f)return;const m=f.value;let p=!1;for(const g of m.layers??[])if(g.imageBlob){const y=jt();i.put(g.imageBlob,y),g.imageBlobRef=y,delete g.imageBlob,p=!0}p&&f.update(m),f.continue()};const d=t.objectStore("project-history").openCursor();d.onsuccess=function(){const f=d.result;if(!f)return;const m=f.value,p=m.entry;let g=!1;switch(p?.type){case"draw":case"transform":case"patch":r(p.before)&&(g=!0),r(p.after)&&(g=!0);break;case"add-layer":case"delete-layer":n(p.layer)&&(g=!0);break;case"crop":for(const y of p.beforeLayers??[])n(y)&&(g=!0);for(const y of p.afterLayers??[])n(y)&&(g=!0);break}g&&f.update(m),f.continue()};const _=t.objectStore("project-stamps").openCursor();_.onsuccess=function(){const f=_.result;if(!f)return;const m=f.value;if(m.blob){const p=jt();i.put(m.blob,p),m.blobRef=p,delete m.blob,f.update(m)}f.continue()}}const Te="blobs";class rc{constructor(t){this._connection=t}async put(t){const s=jt();return await this._tx("readwrite",i=>i.put(t,s)),s}async get(t){const s=await this._tx("readonly",i=>i.get(t));if(s===void 0)throw new Ne(`Blob not found: ${t}`);return s instanceof Blob?s:new Blob([s])}async delete(t){await this._tx("readwrite",s=>s.delete(t))}async deleteMany(t){if(t.length===0)return;const s=await this._connection();await new Promise((i,r)=>{const n=U(s,Te,"readwrite"),a=n.objectStore(Te);for(const o of t)a.delete(o);n.oncomplete=()=>i(),n.onerror=o=>r(mt(o,n)),n.onabort=()=>r(Z(n))})}async gc(t){let s=0;const i=[],r=await this._connection();return await new Promise((n,a)=>{const o=U(r,Te,"readonly"),h=o.objectStore(Te).openKeyCursor();h.onsuccess=()=>{const c=h.result;if(c){const l=c.key;t.has(l)||i.push(l),c.continue()}},o.oncomplete=()=>n(),o.onerror=c=>a(mt(c,o)),o.onabort=()=>a(Z(o))}),i.length>0&&(await this.deleteMany(i),s=i.length),s}async _tx(t,s){const i=await this._connection();return new Promise((r,n)=>{const a=U(i,Te,t),o=s(a.objectStore(Te));t==="readonly"?o.onsuccess=()=>r(o.result):a.oncomplete=()=>r(o.result),a.onerror=h=>n(mt(h,a)),a.onabort=()=>n(Z(a))})}}const Gt="projects";class nc{constructor(t){this._connection=t}async list(t){const s=t?.orderBy??"updatedAt",i=t?.direction??"desc",r=await this._connection();return new Promise((n,a)=>{const o=U(r,Gt,"readonly"),h=o.objectStore(Gt),c=[];if(s==="updatedAt"){const d=h.index("updatedAt").openCursor(null,i==="desc"?"prev":"next");d.onsuccess=()=>{const u=d.result;u?(c.push(u.value),u.continue()):n(c)},d.onerror=()=>a(ht(d.error)),o.onabort=()=>a(Z(o))}else{const l=h.getAll();l.onsuccess=()=>{const d=l.result;d.sort((u,_)=>i==="desc"?_.createdAt-u.createdAt:u.createdAt-_.createdAt),n(d)},l.onerror=()=>a(ht(l.error)),o.onabort=()=>a(Z(o))}})}async get(t){const s=await this._connection();return new Promise((i,r)=>{const n=U(s,Gt,"readonly"),a=n.objectStore(Gt).get(t);a.onsuccess=()=>i(a.result??null),a.onerror=()=>r(ht(a.error)),n.onabort=()=>r(Z(n))})}async create(t){const s=Date.now(),i={id:jt(),name:t.name,createdAt:s,updatedAt:s,thumbnailRef:t.thumbnailRef??null},r=await this._connection();return await new Promise((n,a)=>{const o=U(r,Gt,"readwrite");o.objectStore(Gt).add(i),o.oncomplete=()=>n(),o.onerror=h=>a(mt(h,o)),o.onabort=()=>a(Z(o))}),i}async update(t,s){const i=await this._connection();return new Promise((r,n)=>{const a=U(i,Gt,"readwrite"),o=a.objectStore(Gt);let h;const c=o.get(t);c.onsuccess=()=>{const l=c.result;if(!l){n(new Ne(`Project ${t} not found`));return}s.name!==void 0&&(l.name=s.name),s.thumbnailRef!==void 0&&(l.thumbnailRef=s.thumbnailRef),l.updatedAt=Date.now(),h=l,o.put(l)},c.onerror=()=>n(ht(c.error)),a.oncomplete=()=>r(h),a.onerror=l=>n(mt(l,a)),a.onabort=()=>n(Z(a))})}async delete(t){const s=await this._connection();await new Promise((i,r)=>{const n=U(s,Gt,"readwrite");n.objectStore(Gt).delete(t),n.oncomplete=()=>i(),n.onerror=a=>r(mt(a,n)),n.onabort=()=>r(Z(n))})}}const $e="project-state";class ac{constructor(t){this._connection=t}async get(t){const s=await this._connection();return new Promise((i,r)=>{const n=U(s,$e,"readonly"),a=n.objectStore($e).get(t);a.onsuccess=()=>i(a.result??null),a.onerror=()=>r(ht(a.error)),n.onabort=()=>r(Z(n))})}async save(t){const s=await this._connection();await new Promise((i,r)=>{const n=U(s,$e,"readwrite");n.objectStore($e).put(t),n.oncomplete=()=>i(),n.onerror=a=>r(mt(a,n)),n.onabort=()=>r(Z(n))})}async delete(t){const s=await this._connection();await new Promise((i,r)=>{const n=U(s,$e,"readwrite");n.objectStore($e).delete(t),n.oncomplete=()=>i(),n.onerror=a=>r(mt(a,n)),n.onabort=()=>r(Z(n))})}}const qt="project-history";class oc{constructor(t){this._connection=t}async getEntries(t){const s=await this._connection();return new Promise((i,r)=>{const n=U(s,qt,"readonly"),a=n.objectStore(qt).index("projectId"),o=[],h=a.openCursor(IDBKeyRange.only(t));h.onsuccess=()=>{const c=h.result;c?(o.push(c.value),c.continue()):(o.sort((l,d)=>l.index-d.index),i(o))},h.onerror=()=>r(ht(h.error)),n.onabort=()=>r(Z(n))})}async putEntries(t,s){if(s.length===0)return;const i=await this._connection();await new Promise((r,n)=>{const a=U(i,qt,"readwrite"),o=a.objectStore(qt);for(const h of s){const{id:c,...l}=h;o.add({...l,projectId:t})}a.oncomplete=()=>r(),a.onerror=h=>n(mt(h,a)),a.onabort=()=>n(Z(a))})}async replaceAll(t,s){const i=await this._connection();await new Promise((r,n)=>{const a=U(i,qt,"readwrite"),o=a.objectStore(qt),c=o.index("projectId").openCursor(IDBKeyRange.only(t));c.onsuccess=()=>{const l=c.result;if(l)l.delete(),l.continue();else for(const d of s){const{id:u,..._}=d;o.add({..._,projectId:t})}},c.onerror=()=>n(ht(c.error)),a.oncomplete=()=>r(),a.onerror=l=>n(mt(l,a)),a.onabort=()=>n(Z(a))})}async updateEntries(t,s,i){if(s.length===0&&i.length===0)return;const r=await this._connection();await new Promise((n,a)=>{const o=U(r,qt,"readwrite"),h=o.objectStore(qt);if(s.length>0){const c=new Set(s),l=h.index("projectId").openCursor(IDBKeyRange.only(t));l.onsuccess=()=>{const d=l.result;d&&(c.has(d.value.index)&&d.delete(),d.continue())},l.onerror=()=>a(ht(l.error))}for(const c of i){const{id:l,...d}=c;h.add({...d,projectId:t})}o.oncomplete=()=>n(),o.onerror=c=>a(mt(c,o)),o.onabort=()=>a(Z(o))})}async deleteForProject(t){const s=await this._connection();await new Promise((i,r)=>{const n=U(s,qt,"readwrite"),o=n.objectStore(qt).index("projectId").openCursor(IDBKeyRange.only(t));o.onsuccess=()=>{const h=o.result;h&&(h.delete(),h.continue())},o.onerror=()=>r(ht(o.error)),n.oncomplete=()=>i(),n.onerror=h=>r(mt(h,n)),n.onabort=()=>r(Z(n))})}}const Kt="project-stamps";class hc{constructor(t,s){this._connection=t,this._blobs=s}async list(t){const s=await this._connection();return new Promise((i,r)=>{const n=U(s,Kt,"readonly"),a=n.objectStore(Kt).index("projectId"),o=[],h=a.openCursor(IDBKeyRange.only(t));h.onsuccess=()=>{const c=h.result;c?(o.push(c.value),c.continue()):(o.sort((l,d)=>d.createdAt-l.createdAt),i(o))},h.onerror=()=>r(ht(h.error)),n.onabort=()=>r(Z(n))})}async add(t,s,i=Date.now()){const r=await this._blobs.put(s),n={id:jt(),projectId:t,blobRef:r,createdAt:i},a=await this._connection();return await new Promise((o,h)=>{const c=U(a,Kt,"readwrite");c.objectStore(Kt).add(n),c.oncomplete=()=>o(),c.onerror=l=>h(mt(l,c)),c.onabort=()=>h(Z(c))}),n}async delete(t){const s=await this._connection(),i=await new Promise((n,a)=>{const o=U(s,Kt,"readonly"),h=o.objectStore(Kt).get(t);h.onsuccess=()=>{const c=h.result;n(c?.blobRef??null)},h.onerror=()=>a(ht(h.error)),o.onabort=()=>a(Z(o))}),r=await this._connection();await new Promise((n,a)=>{const o=U(r,Kt,"readwrite");o.objectStore(Kt).delete(t),o.oncomplete=()=>n(),o.onerror=h=>a(mt(h,o)),o.onabort=()=>a(Z(o))}),i&&this._blobs.delete(i).catch(()=>{})}async deleteForProject(t){const s=[],i=await this._connection();await new Promise((r,n)=>{const a=U(i,Kt,"readwrite"),h=a.objectStore(Kt).index("projectId").openCursor(IDBKeyRange.only(t));h.onsuccess=()=>{const c=h.result;if(c){const l=c.value;s.push(l.blobRef),c.delete(),c.continue()}},h.onerror=()=>n(ht(h.error)),a.oncomplete=()=>r(),a.onerror=c=>n(mt(c,a)),a.onabort=()=>n(Z(a))}),s.length>0&&this._blobs.deleteMany(s).catch(()=>{})}}async function Si(e,t){const s=await new Response(new Blob([e]).stream().pipeThrough(t)).arrayBuffer();return new Uint8Array(s)}function cc(e,t){if(e.length!==t.length)return!1;for(let s=0;s<e.length;s++)if(e[s]!==t[s])return!1;return!0}async function Or(e){const t={encoding:"raw",data:e.slice().buffer};if(typeof CompressionStream>"u"||typeof DecompressionStream>"u")return t;try{const s=await Si(e,new CompressionStream("deflate"));if(s.length>=e.length)return t;const i=await Si(s,new DecompressionStream("deflate"));return cc(i,e)?{encoding:"deflate",data:s.slice().buffer}:t}catch{return t}}async function Br(e,t){if(Object.prototype.toString.call(t)!=="[object ArrayBuffer]")return null;if(e==="raw")return new Uint8Array(t);if(e!=="deflate"||typeof DecompressionStream>"u")return null;try{return await Si(new Uint8Array(t),new DecompressionStream("deflate"))}catch{return null}}const me="brush-tips",ge="brush-tip-masks",ye="brush-presets",Ve="brush-patterns",ds=[me,ge,ye,Ve];function us(e){return new Promise((t,s)=>{e.oncomplete=()=>t(),e.onerror=i=>s(mt(i,e)),e.onabort=()=>s(Z(e))})}function lc(e){const t=e;return!!t&&typeof t.id=="string"&&typeof t.name=="string"&&ts(t.thumb)&&typeof t.spacing=="number"&&typeof t.createdAt=="number"}function dc(e){const t=e;return!!t&&typeof t.id=="string"&&typeof t.name=="string"&&!!t.descriptor&&typeof t.descriptor=="object"}class uc{constructor(t){this._connection=t}async list(){const t=await this._connection();return new Promise((s,i)=>{const r=U(t,[me,ye],"readonly"),n=r.objectStore(me).getAll(),a=r.objectStore(ye).getAll();n.onerror=()=>i(ht(n.error)),a.onerror=()=>i(ht(a.error)),r.oncomplete=()=>{const o=n.result.filter(lc).map(c=>({...c,thumb:{...c.thumb,alpha:new Uint8Array(c.thumb.alpha)}})).sort((c,l)=>c.createdAt-l.createdAt),h=a.result.filter(dc).sort((c,l)=>c.createdAt-l.createdAt);s({tips:o,presets:h})},r.onabort=()=>i(Z(r))})}async getMask(t){const s=await this._connection(),i=await new Promise((a,o)=>{const h=U(s,ge,"readonly"),c=h.objectStore(ge).get(t);c.onsuccess=()=>a(c.result),c.onerror=()=>o(ht(c.error)),h.onabort=()=>o(Z(h))});if(!i)return null;const r=await Br(i.encoding,i.data),n=r&&{width:i.width,height:i.height,alpha:r};return ts(n)?n:null}async listPatterns(){const t=await this._connection(),s=await new Promise((r,n)=>{const a=U(t,Ve,"readonly"),o=a.objectStore(Ve).getAll();o.onsuccess=()=>r(o.result),o.onerror=()=>n(ht(o.error)),a.onabort=()=>n(Z(a))}),i=[];for(const r of s){if(!r||typeof r.id!="string"||typeof r.name!="string"||typeof r.createdAt!="number")continue;const n=await Br(r.encoding,r.data);!n||!Number.isInteger(r.width)||!Number.isInteger(r.height)||r.width<1||r.height<1||n.length!==r.width*r.height||i.push({id:r.id,name:r.name,width:r.width,height:r.height,createdAt:r.createdAt,data:n})}return i.sort((r,n)=>r.createdAt-n.createdAt)}async deleteMany(t){const s=await this._connection(),i=U(s,ds,"readwrite"),r=us(i),n=i.objectStore(ye);for(const o of t.presets??[])n.delete(o);for(const o of t.tips??[])i.objectStore(me).delete(o),i.objectStore(ge).delete(o);const a=[];if(t.unusedPatterns?.length){const o=n.getAll();o.onsuccess=()=>{const h=new Set(o.result.map(c=>c?.descriptor?.texture?.pattern));for(const c of t.unusedPatterns)h.has(c)||(i.objectStore(Ve).delete(c),a.push(c))}}return await r,{patterns:a}}async putBatch(t){const s=[];for(const{meta:l,mask:d}of t.tips??[]){const u=await Or(d.alpha);s.push({id:l.id,width:d.width,height:d.height,...u})}const i=[];for(const l of t.patterns??[]){const d=await Or(l.data);i.push({id:l.id,name:l.name,width:l.width,height:l.height,createdAt:l.createdAt,...d})}const r=await this._connection(),n=U(r,ds,"readwrite"),a=us(n);for(const l of i)n.objectStore(Ve).put(l);const o=n.objectStore(me),h=n.objectStore(ge);(t.tips??[]).forEach(({meta:l},d)=>{o.put(l),h.put(s[d])});const c=n.objectStore(ye);for(const l of t.presets??[])c.put(l);await a}async deletePreset(t){const s=await this._connection(),i=U(s,ds,"readwrite"),r=us(i),n=i.objectStore(ye),a=n.get(t);a.onsuccess=()=>{const o=a.result?.tipId;if(n.delete(t),!o)return;const h=n.index("tipId").count(IDBKeyRange.only(o));h.onsuccess=()=>{h.result===0&&(i.objectStore(me).delete(o),i.objectStore(ge).delete(o))}},await r}async deleteTip(t){const s=await this._connection(),i=U(s,ds,"readwrite"),r=us(i);i.objectStore(me).delete(t),i.objectStore(ge).delete(t);const n=i.objectStore(ye).index("tipId").openCursor(IDBKeyRange.only(t));n.onsuccess=()=>{const a=n.result;a&&(a.delete(),a.continue())},await r}}const pc=1e4,fc=1e4,_c="ketchup-projects",mc=8;class gc{constructor(t){this._db=null,this._opening=null,this._versionChanged=!1,this._writes=new Set,this._closingFor=null,this._connection=()=>this._db?Promise.resolve(this._db):this._opening?this._opening:Promise.reject(this._closedError()),this._dbName=t?.dbName??_c,this._version=t?.version??mc,this._onBlocked=t?.onBlocked,this._onVersionChange=t?.onVersionChange}get projects(){if(!this._projects)throw new At("Backend not initialized — call init() first");return this._projects}get state(){if(!this._state)throw new At("Backend not initialized — call init() first");return this._state}get history(){if(!this._history)throw new At("Backend not initialized — call init() first");return this._history}get stamps(){if(!this._stamps)throw new At("Backend not initialized — call init() first");return this._stamps}get brushes(){if(!this._brushes)throw new At("Backend not initialized — call init() first");return this._brushes}get blobs(){if(!this._blobs)throw new At("Backend not initialized — call init() first");return this._blobs}async writeTogether(t){if(this._closingFor&&this._closingFor===this._db)throw this._closedError(!0);const s=t(),i=s.then(()=>{},()=>{});this._writes.add(i);try{return await s}finally{this._writes.delete(i)}}_writesSettled(){return this._writes.size===0?Promise.resolve():new Promise(t=>{const s=setTimeout(t,fc);Promise.all([...this._writes]).then(()=>{clearTimeout(s),t()})})}_closedError(t=this._versionChanged){return t?new Le("Storage was upgraded by another window; reload to use it",!0):new Le("Storage is closed")}async init(){if(this._db?.close(),this._db=null,this._versionChanged)throw this._closedError();const t=this._open();this._opening=t;try{this._db=await t}catch(i){throw i?.name==="VersionError"?(this._versionChanged=!0,new Le("Storage was upgraded by another window; reload to use it",!0,i)):i}finally{this._opening===t&&(this._opening=null)}const s=this._db;if(s.onversionchange=()=>{let i=!1;const r=()=>{if(i)return;i=!0;const o=()=>{s.close(),this._closingFor===s&&(this._closingFor=null),this._db===s&&(this._db=null,this._versionChanged=!0)};if(this._writes.size===0){o();return}this._closingFor=s,this._writesSettled().then(o)};let n;try{n=this._onVersionChange?.()}catch(o){console.error("onVersionChange failed:",o)}if(!n){r();return}const a=setTimeout(r,pc);Promise.resolve(n).catch(o=>console.error("onVersionChange failed:",o)).then(()=>{clearTimeout(a),r()})},!this._blobs){const i=new rc(this._connection);this._blobs=i,this._projects=new nc(this._connection),this._state=new ac(this._connection),this._history=new oc(this._connection),this._stamps=new hc(this._connection,i),this._brushes=new uc(this._connection)}}_open(){const t=indexedDB.deleteDatabase("ketchup-stamps");return t.onerror=()=>{},t.onblocked=()=>{},new Promise((s,i)=>{const r=indexedDB.open(this._dbName,this._version);r.onupgradeneeded=n=>{const a=r.result,o=r.transaction,h=n.oldVersion;a.objectStoreNames.contains("projects")||a.createObjectStore("projects",{keyPath:"id"}).createIndex("updatedAt","updatedAt"),a.objectStoreNames.contains("project-state")||a.createObjectStore("project-state",{keyPath:"projectId"}),a.objectStoreNames.contains("project-history")||a.createObjectStore("project-history",{keyPath:"id",autoIncrement:!0}).createIndex("projectId","projectId"),a.objectStoreNames.contains("project-stamps")||a.createObjectStore("project-stamps",{keyPath:"id"}).createIndex("projectId","projectId"),a.objectStoreNames.contains("brush-tips")||a.createObjectStore("brush-tips",{keyPath:"id"}),a.objectStoreNames.contains("brush-tip-masks")||a.createObjectStore("brush-tip-masks",{keyPath:"id"}),a.objectStoreNames.contains("brush-presets")||a.createObjectStore("brush-presets",{keyPath:"id"}).createIndex("tipId","tipId"),a.objectStoreNames.contains("brush-patterns")||a.createObjectStore("brush-patterns",{keyPath:"id"}),h<4&&!a.objectStoreNames.contains("blobs")&&ic(a,o,h)},r.onblocked=()=>{console.warn("Storage upgrade is waiting for another tab to close"),this._onBlocked?.(!0)},r.onsuccess=()=>{this._onBlocked?.(!1),s(r.result)},r.onerror=()=>{this._onBlocked?.(!1),i(r.error)}})}async dispose(){this._db&&(this._db.close(),this._db=null)}}async function cr(e){return new Promise((t,s)=>{e.toBlob(i=>{i?t(i):s(new Error("canvas.toBlob returned null"))},"image/png")})}const ya=[75,84,67,72],va=1,Rs=16,Hr=16384;class ke extends Error{constructor(t,s){super(t),this.cause=s,this.name="PixelDecodeError"}}function yc(e){const t=e?.name;return t==="NotFoundError"||t==="NotReadableError"}function ba(){return typeof DecompressionStream<"u"&&typeof Blob<"u"&&typeof Blob.prototype.stream=="function"}function vc(){return typeof CompressionStream<"u"&&ba()}const bc=3e3;let ii=null,ps=null;const wc=6e4;let ri=-1/0;function xc(){if(ii!==null)return Promise.resolve(ii);if(!vc())return Promise.resolve(!1);if(!ps){if(Date.now()-ri<wc)return Promise.resolve(!1);const e=new Promise(t=>{queueMicrotask(()=>{const s=new AbortController;let i=!1;const r=setTimeout(()=>{i=!0,ri=Date.now(),s.abort(),t(!1)},bc);kc(s.signal).then(n=>{i||(i=!0,clearTimeout(r),n!==null?ii=n:ri=Date.now(),t(n===!0))})})});ps=e,e.then(()=>{ps=null})}return ps}async function Fr(e,t){const s=e.getReader(),i=()=>{s.cancel().catch(()=>{})};t.addEventListener("abort",i);try{const r=[];let n=0;for(;;){const{done:h,value:c}=await s.read();if(h)break;r.push(c),n+=c.length}const a=new Uint8Array(n);let o=0;for(const h of r)a.set(h,o),o+=h.length;return a}finally{t.removeEventListener("abort",i)}}async function kc(e){try{const s=new Uint8Array(65536);let i=12345;const r=()=>i=Math.imul(i,1664525)+1013904223>>>0;let n=0;for(;n<4096;n++)s[n]=r()>>>24;for(;n<65536;){const h=4+(r()>>>29),c=n-1-r()%Math.min(n-1,3e4);for(let l=0;l<h&&n<65536;l++)s[n++]=s[c+l]}const a=await Fr(new Blob([s]).stream().pipeThrough(new CompressionStream("deflate"),{signal:e}),e);if(e.aborted)return null;let o;try{o=await Fr(new Blob([a]).stream().pipeThrough(new DecompressionStream("deflate"),{signal:e}),e)}catch{return e.aborted?null:!1}if(e.aborted)return null;if(o.length!==65536)return!1;for(let h=0;h<65536;h++)if(o[h]!==s[h])return!1;return!0}catch{return null}}function Mc(e,t){if(!(e>0&&t>0)||e>Hr||t>Hr)throw new ke(`Stored pixel data has invalid dimensions ${e}x${t}`)}async function Cc(e){const t=new ArrayBuffer(Rs),s=new DataView(t);ya.forEach((n,a)=>s.setUint8(a,n)),s.setUint32(4,va),s.setUint32(8,e.width),s.setUint32(12,e.height);const i=new Blob([e.data]).stream().pipeThrough(new CompressionStream("deflate")),r=await new Response(i).blob();return new Blob([t,r],{type:"application/octet-stream"})}async function wa(e){if(e.size<Rs)return null;const t=new DataView(await e.slice(0,Rs).arrayBuffer());for(let s=0;s<4;s++)if(t.getUint8(s)!==ya[s])return null;if(t.getUint32(4)!==va)throw new Error("Unsupported stored pixel format version");return{width:t.getUint32(8),height:t.getUint32(12)}}async function Pi(e,t,s){if(!ba())throw new Error("DecompressionStream is unavailable");Mc(t,s);const i=new Uint8ClampedArray(t*s*4),r=e.slice(Rs).stream().pipeThrough(new DecompressionStream("deflate")).getReader();let n=0;try{for(;;){const{done:a,value:o}=await r.read();if(a)break;if(n+o.length>i.length)throw new ke("Stored pixel data is larger than expected");i.set(o,n),n+=o.length}if(n!==i.length)throw new ke("Stored pixel data is truncated")}catch(a){throw await r.cancel().catch(()=>{}),a instanceof ke||yc(a)?a:new ke("Stored pixel data is not valid deflate",a)}return new ImageData(i,t,s)}async function xa(e){try{return await createImageBitmap(e)}catch(t){throw t?.name!=="InvalidStateError"?t:new ke("Stored image could not be decoded",t)}}async function Sc(e,t,s){const i=await wa(e);if(i){if(i.width===t&&i.height===s)return Pi(e,t,s);const o=await Pi(e,i.width,i.height),h=document.createElement("canvas");h.width=t,h.height=s;const c=h.getContext("2d");return c.putImageData(o,0,0),c.getImageData(0,0,t,s)}const r=await xa(e),n=document.createElement("canvas");n.width=t,n.height=s;const a=n.getContext("2d");return a.drawImage(r,0,0),r.close(),a.getImageData(0,0,t,s)}async function Pc(e,t,s){const i=document.createElement("canvas");i.width=t,i.height=s;const r=i.getContext("2d"),n=await wa(e);if(n)return r.putImageData(await Pi(e,n.width,n.height),0,0),i;const a=await xa(e);return r.drawImage(a,0,0),a.close(),i}const Tc=.25;function $c(e){const t=e.data;for(let s=3;s<t.length;s+=4)if(t[s]!==255)return!1;return!0}async function ka(e){if(await xc()){const t=await Cc(e);if(t.size>e.data.length*Tc&&$c(e)){const s=await Nr(e).catch(()=>null);if(s&&s.size<t.size)return s}return t}return Nr(e)}function Nr(e){const t=document.createElement("canvas");t.width=e.width,t.height=e.height,t.getContext("2d").putImageData(e,0,0);const i=()=>{t.width=0,t.height=0};return cr(t).finally(i)}function Ti(e){return new Uint32Array(e.data.buffer,e.data.byteOffset,e.width*e.height)}function Ge(e,t){const s=e.width,i=e.height,r=Ti(e),n=Ti(t);let a=s,o=-1,h=-1,c=-1;for(let l=0;l<i;l++){const d=l*s;let u=0;for(;u<s&&r[d+u]===n[d+u];)u++;if(u===s)continue;h<0&&(h=l),c=l,u<a&&(a=u);let _=s-1;for(;_>o&&r[d+_]===n[d+_];)_--;_>o&&(o=_)}return h<0?null:{x:a,y:h,w:o-a+1,h:c-h+1}}function Zt(e,t){if(t.x===0&&t.y===0&&t.w===e.width&&t.h===e.height)return e;const s=new ImageData(t.w,t.h),i=t.w*4;for(let r=0;r<t.h;r++){const n=((t.y+r)*e.width+t.x)*4;s.data.set(e.data.subarray(n,n+i),r*i)}return s}function ni(e){const t=Ti(e);let s=2166136261,i=2654435769;for(let r=0;r<t.length;r++){const n=t[r];s=Math.imul(s^n,16777619),s^=s>>>13,i=Math.imul(i^n,1540483477),i^=i>>>15}return`${e.width}x${e.height}:${(s>>>0).toString(16)}:${(i>>>0).toString(16)}`}const Xr=1024*1024;function Ma(){const e=(typeof navigator<"u"?navigator.deviceMemory:void 0)??4;return Math.max(64*Xr,Math.round(e*128)*Xr)}const ai=e=>e.imageData.data.byteLength;function Wr(e){switch(e.type){case"draw":case"patch":case"transform":return e.before.data.byteLength+e.after.data.byteLength;case"add-layer":case"delete-layer":return ai(e.layer);case"crop":case"merge":return e.beforeLayers.reduce((t,s)=>t+ai(s),0)+e.afterLayers.reduce((t,s)=>t+ai(s),0);default:return 0}}const $i=e=>{const t=e.width*e.height*4;return Number.isFinite(t)&&t>0?t:0},oi=e=>$i(e.imageData);function Yr(e){try{return Ic(e)}catch{return 0}}function Ic(e){switch(e.type){case"draw":case"patch":case"transform":return $i(e.before)+$i(e.after);case"add-layer":case"delete-layer":return oi(e.layer);case"crop":case"merge":return e.beforeLayers.reduce((t,s)=>t+oi(s),0)+e.afterLayers.reduce((t,s)=>t+oi(s),0);default:return 0}}const Ur=16384;class Ca extends Error{constructor(){super(...arguments),this.name="MalformedRecordError"}}function Ce(e){throw new Ca(`Stored history record is malformed: ${e}`)}function lr(e){return typeof e=="object"&&e!==null}function Ii(e){lr(e)||Ce("missing pixel data"),(typeof e.blobRef!="string"||!e.blobRef)&&Ce("missing blob reference");const{width:t,height:s}=e;(!Number.isInteger(t)||!Number.isInteger(s)||t<=0||s<=0||t>Ur||s>Ur)&&Ce(`invalid size ${String(t)}x${String(s)}`)}function Sa(e){lr(e)||Ce("missing layer"),Ii(e.imageData)}function Vr(e){Array.isArray(e)||Ce("missing layer list"),e.forEach(Sa)}function Ec(e){switch(lr(e)||Ce("not an object"),e.type){case"draw":case"patch":case"transform":Ii(e.before),Ii(e.after);return;case"add-layer":case"delete-layer":Sa(e.layer);return;case"crop":case"merge":Vr(e.beforeLayers),Vr(e.afterLayers);return;case"reorder":case"visibility":case"opacity":case"rename":case"blend-mode":case"alpha-lock":return;default:Ce(`unknown type ${String(e.type)}`)}}async function ve(e,t){const s=await ka(e),i=await t.put(s);return{width:e.width,height:e.height,blobRef:i}}async function be(e,t){const s=await t.get(e.blobRef);return Sc(s,e.width,e.height)}async function Ie(e,t){return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode,alphaLocked:e.alphaLocked,imageData:await ve(e.imageData,t)}}async function Ee(e,t){return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode??"normal",alphaLocked:e.alphaLocked===!0,imageData:await be(e.imageData,t)}}async function Rc(e,t,s){const i=await ka(t),r=await s.put(i);return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode??"normal",alphaLocked:e.alphaLocked===!0,imageBlobRef:r}}async function Dc(e,t,s,i){const r=await i.get(e.imageBlobRef),n=await Pc(r,t,s);return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode??"normal",alphaLocked:e.alphaLocked===!0,canvas:n}}async function Ac(e,t){switch(e.type){case"draw":{const[s,i]=await Promise.all([ve(e.before,t),ve(e.after,t)]);return{type:"draw",layerId:e.layerId,before:s,after:i}}case"patch":{const[s,i]=await Promise.all([ve(e.before,t),ve(e.after,t)]);return{type:"patch",layerId:e.layerId,x:e.x,y:e.y,before:s,after:i}}case"add-layer":return{type:"add-layer",layer:await Ie(e.layer,t),index:e.index};case"delete-layer":return{type:"delete-layer",layer:await Ie(e.layer,t),index:e.index};case"crop":{const[s,i]=await Promise.all([Promise.all(e.beforeLayers.map(r=>Ie(r,t))),Promise.all(e.afterLayers.map(r=>Ie(r,t)))]);return{type:"crop",beforeLayers:s,afterLayers:i,beforeWidth:e.beforeWidth,beforeHeight:e.beforeHeight,afterWidth:e.afterWidth,afterHeight:e.afterHeight}}case"merge":{const[s,i]=await Promise.all([Promise.all(e.beforeLayers.map(r=>Ie(r,t))),Promise.all(e.afterLayers.map(r=>Ie(r,t)))]);return{type:"merge",beforeLayers:s,afterLayers:i,previousActiveLayerId:e.previousActiveLayerId,afterActiveLayerId:e.afterActiveLayerId}}case"reorder":case"visibility":case"opacity":case"rename":case"blend-mode":case"alpha-lock":return e;case"transform":{const[s,i]=await Promise.all([ve(e.before,t),ve(e.after,t)]);return{type:"transform",layerId:e.layerId,before:s,after:i}}}}async function Lc(e,t){switch(Ec(e),e.type){case"draw":{const[s,i]=await Promise.all([be(e.before,t),be(e.after,t)]);return{type:"draw",layerId:e.layerId,before:s,after:i}}case"patch":{const[s,i]=await Promise.all([be(e.before,t),be(e.after,t)]);return{type:"patch",layerId:e.layerId,x:e.x,y:e.y,before:s,after:i}}case"add-layer":return{type:"add-layer",layer:await Ee(e.layer,t),index:e.index};case"delete-layer":return{type:"delete-layer",layer:await Ee(e.layer,t),index:e.index};case"crop":{const[s,i]=await Promise.all([Promise.all(e.beforeLayers.map(r=>Ee(r,t))),Promise.all(e.afterLayers.map(r=>Ee(r,t)))]);return{type:"crop",beforeLayers:s,afterLayers:i,beforeWidth:e.beforeWidth,beforeHeight:e.beforeHeight,afterWidth:e.afterWidth,afterHeight:e.afterHeight}}case"merge":{const[s,i]=await Promise.all([Promise.all(e.beforeLayers.map(r=>Ee(r,t))),Promise.all(e.afterLayers.map(r=>Ee(r,t)))]);return{type:"merge",beforeLayers:s,afterLayers:i,previousActiveLayerId:e.previousActiveLayerId,afterActiveLayerId:e.afterActiveLayerId}}case"reorder":case"visibility":case"opacity":case"rename":case"alpha-lock":return e;case"blend-mode":return{type:"blend-mode",layerId:e.layerId,before:e.before,after:e.after};case"transform":{const[s,i]=await Promise.all([be(e.before,t),be(e.after,t)]);return{type:"transform",layerId:e.layerId,before:s,after:i}}}}const qe={select:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square" stroke-linejoin="miter">
      <rect x="4" y="4" width="16" height="16" stroke-dasharray="4 4" />
    </svg>`,move:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="5 9 2 12 5 15"/>
      <polyline points="9 5 12 2 15 5"/>
      <polyline points="15 19 12 22 9 19"/>
      <polyline points="19 9 22 12 19 15"/>
      <line x1="2" y1="12" x2="22" y2="12"/>
      <line x1="12" y1="2" x2="12" y2="22"/>
    </svg>`,pencil:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M17 3a2.83 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>
    </svg>`,eraser:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m7 21-4.3-4.3c-1-1-1-2.5 0-3.4l9.6-9.6c1-1 2.5-1 3.4 0l5.6 5.6c1 1 1 2.5 0 3.4L13 21"/>
      <path d="M22 21H7"/>
      <path d="m5 11 9 9"/>
    </svg>`,line:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <line x1="5" y1="19" x2="19" y2="5"/>
    </svg>`,rectangle:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2"/>
    </svg>`,circle:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="12" cy="12" r="10"/>
    </svg>`,triangle:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 3L22 21H2z"/>
    </svg>`,diamond:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2L22 12 12 22 2 12z"/>
    </svg>`,pentagon:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2L22 9.3 18.2 21H5.8L2 9.3z"/>
    </svg>`,hexagon:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M7 2H17L22 12 17 22H7L2 12z"/>
    </svg>`,star:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2.5l2.9 5.9 6.5.9-4.7 4.6 1.1 6.5-5.8-3-5.8 3 1.1-6.5-4.7-4.6 6.5-.9z"/>
    </svg>`,heart:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8l1.1 1.1L12 21.2l7.8-7.7 1.1-1.1a5.5 5.5 0 0 0-.1-7.8z"/>
    </svg>`,arrow:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 9h10V4l8 8-8 8v-5H3z"/>
    </svg>`,fill:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m19 11-8-8-8.6 8.6a2 2 0 0 0 0 2.8l5.2 5.2c.8.8 2 .8 2.8 0L19 11Z"/>
      <path d="m5 2 5 5"/>
      <path d="M2 13h15"/>
      <path d="M22 20a2 2 0 1 1-4 0c0-1.6 1.7-2.4 2-4 .3 1.6 2 2.4 2 4Z"/>
    </svg>`,stamp:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M2 22h20"/>
      <path d="M6 12v-4c0-2.2 1.8-4 4-4h4c2.2 0 4 1.8 4 4v4"/>
      <path d="M6 12h12a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2v-2a2 2 0 0 1 2-2Z"/>
      <path d="M9 13v-1"/>
      <path d="M15 13v-1"/>
    </svg>`,eyedropper:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m2 22 1-1h3l9-9"/>
      <path d="M3 21v-3l9-9"/>
      <path d="m15 6 3.4-3.4a2.1 2.1 0 1 1 3 3L18 9"/>
      <path d="m15 6 3 3"/>
    </svg>`,text:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="4 7 4 4 20 4 20 7"/>
      <line x1="12" y1="4" x2="12" y2="21"/>
      <line x1="8" y1="21" x2="16" y2="21"/>
    </svg>`,hand:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M18 11V6a2 2 0 0 0-4 0v1"/>
      <path d="M14 10V4a2 2 0 0 0-4 0v6"/>
      <path d="M10 10.5V6a2 2 0 0 0-4 0v8"/>
      <path d="M18 8a2 2 0 0 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/>
    </svg>`,crop:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M6 2v4H2"/>
      <path d="M6 6h12v12"/>
      <path d="M18 22v-4h4"/>
      <path d="M2 6h4v12h12"/>
    </svg>`},Gr=W`
  <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M12 2.5 17 10H7z"/>
    <circle cx="6.5" cy="17" r="4"/>
    <rect x="13" y="13" width="8" height="8" rx="1"/>
  </svg>`,Tt={undo:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="1 4 1 10 7 10"/>
      <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/>
    </svg>`,redo:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="23 4 23 10 17 10"/>
      <path d="M20.49 15a9 9 0 1 1-2.13-9.36L23 10"/>
    </svg>`,save:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
      <polyline points="7 10 12 15 17 10"/>
      <line x1="12" y1="15" x2="12" y2="3"/>
    </svg>`,clear:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 6h18"/>
      <path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/>
      <path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>
    </svg>`,exitChildMode:W`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/>
      <polyline points="10 17 15 12 10 7"/>
      <line x1="15" y1="12" x2="3" y2="12"/>
    </svg>`},dr={select:"V",move:"M",hand:"H",pencil:"B",eraser:"E",line:"L",rectangle:"U",circle:"C",triangle:"T",diamond:"D",pentagon:"P",hexagon:"K",star:"A",heart:"J",arrow:"Q",fill:"G",stamp:"S",text:"X",crop:"R",eyedropper:"I"},jc=new Map(Object.entries(dr).map(([e,t])=>[t.toLowerCase(),e]));function zc(e){return jc.get(e.toLowerCase())}const oe={select:"Select",move:"Move",pencil:"Pencil",eraser:"Eraser",line:"Line",rectangle:"Rectangle",circle:"Circle",triangle:"Triangle",diamond:"Diamond",pentagon:"Pentagon",hexagon:"Hexagon",star:"Star",heart:"Heart",arrow:"Arrow",fill:"Fill",stamp:"Stamp",text:"Text",hand:"Hand (Pan)",crop:"Crop",eyedropper:"Eyedropper"},Pa=["pencil","eraser","rectangle","circle","triangle","fill"],Oc=new Set(Pa),Ei=120,Ri=8,Di=1600;function qr(e,t=Ei){return Number.isFinite(e)?Math.max(Ri,Math.min(Di,e)):t}function Ai(e){return(e%360+360)%360}function Ta(e,t,s){const i=Ai(t-e+180)-180;return Ai(e+i*s)}const Bc=120;function Li(e,t=0){return Number.isFinite(e)?Math.max(0,Math.min(100,e)):t}function Hc(e){return Li(e)/100*Bc}const Fc=64,Nc=1e3/60;class Xc{constructor(){this._tau=0,this._x=0,this._y=0,this._t=0,this._prevX=0,this._prevY=0,this._prevT=0,this._rawX=0,this._rawY=0,this._rawPressure=0,this._rawTiltX=0,this._rawTiltY=0,this._rawTwist=0,this._rawS=0,this._anchorX=0,this._anchorY=0,this._anchorS=0,this._slack=0,this._penS=0,this._history=[],this._started=!1}reset(t=this._tau,s=this._slack){this._tau=t,this._slack=Math.max(0,s),this._started=!1,this._history=[]}add(t,s,i,r,n=0,a=0,o=0){if(this._rawPressure=i,this._rawTiltX=n,this._rawTiltY=a,this._rawTwist=o,!this._started||this._tau<=0)return this._rawX=t,this._rawY=s,this._x=t,this._y=s,this._t=r,this._prevX=t,this._prevY=s,this._prevT=r,this._anchorX=t,this._anchorY=s,this._anchorS=0,this._rawS=0,this._penS=0,this._history=[{s:0,pressure:i,tiltX:n,tiltY:a,twist:o}],this._started=!0,{x:t,y:s,pressure:i,tiltX:n,tiltY:a,twist:o};let h=Math.hypot(t-this._anchorX,s-this._anchorY);if(h>this._slack){const d=(h-this._slack)/h;this._anchorX+=(t-this._anchorX)*d,this._anchorY+=(s-this._anchorY)*d,this._anchorS+=h-this._slack,h=this._slack}this._rawS=Math.max(this._rawS,this._anchorS+h),this._rawX=t,this._rawY=s,this._remember({s:this._rawS,pressure:i,tiltX:n,tiltY:a,twist:o});const c=r-this._t;if(c>0){this._prevX=this._x,this._prevY=this._y,this._prevT=this._t;const d=1-Math.exp(-c/this._tau);this._x+=(t-this._x)*d,this._y+=(s-this._y)*d,this._penS+=(this._rawS-this._penS)*d,this._t=r}this._trim();const l=this._penAt(this._penS);return{x:this._x,y:this._y,pressure:l.pressure,tiltX:l.tiltX,tiltY:l.tiltY,twist:l.twist}}lag(){return!this._started||this._tau<=0?0:Math.hypot(this._rawX-this._x,this._rawY-this._y)}snap(t){return this._prevX=this._x,this._prevY=this._y,this._prevT=this._t,this._x=this._rawX,this._y=this._rawY,this._t=Math.max(this._t,t),this._penS=this._rawS,this._trim(),{x:this._x,y:this._y,...this.rawPen()}}penTrail(){const t=this._history.slice(),s=this._penS;return i=>{const r=this._history;this._history=t;const n=this._penAt(s+(this._penS-s)*i);return this._history=r,{pressure:n.pressure,tiltX:n.tiltX,tiltY:n.tiltY,twist:n.twist}}}setPrevious(t,s,i){this._prevX=t,this._prevY=s,this._prevT=i}position(){return{x:this._x,y:this._y}}rawPen(){return{pressure:this._rawPressure,tiltX:this._rawTiltX,tiltY:this._rawTiltY,twist:this._rawTwist}}hold(t){return this.add(this._rawX,this._rawY,this._rawPressure,t,this._rawTiltX,this._rawTiltY,this._rawTwist)}catchUp(){if(!this._started||this._tau<=0)return[];const t=this._rawX-this._x,s=this._rawY-this._y,i=Math.hypot(t,s);if(i<.01)return[];const r=Math.hypot(this._x-this._prevX,this._y-this._prevY),n=this._t-this._prevT,a=r>0&&n>0?r/n:0,o=Math.min(Fc,Math.ceil(i/Math.max(1,r))),h=a>0?i/o/a:Nc,c=this._penS,l=this._rawS-c,d={x:this._x,y:this._y,t:this._t},u=[];for(let f=1;f<=o;f++){const m=f/o,p=f===o?this._history[this._history.length-1]:this._penAt(c+l*m);u.push({x:d.x+t*m,y:d.y+s*m,pressure:p.pressure,tiltX:p.tiltX,tiltY:p.tiltY,twist:p.twist,timestamp:d.t+h*f})}const _=u[o-1];return this._prevX=o>1?u[o-2].x:d.x,this._prevY=o>1?u[o-2].y:d.y,this._prevT=_.timestamp-h,this._x=_.x,this._y=_.y,this._t=_.timestamp,this._penS=this._rawS,this._trim(),u}_remember(t){const s=this._history,i=s.length;i>=2&&s[i-1].s===t.s&&s[i-2].s===t.s?s[i-1]=t:s.push(t)}_trim(){const t=this._history;let s=0;for(;s+1<t.length&&t[s+1].s<=this._penS;)s++;s>0&&t.splice(0,s)}_penAt(t){const s=this._history;let i=0;for(;i+1<s.length&&s[i+1].s<=t;)i++;if(i+1>=s.length)return s[s.length-1];const r=s[i],n=s[i+1];if(!(t>r.s))return r;const a=(t-r.s)/(n.s-r.s);return{s:t,pressure:r.pressure+(n.pressure-r.pressure)*a,tiltX:r.tiltX+(n.tiltX-r.tiltX)*a,tiltY:r.tiltY+(n.tiltY-r.tiltY)*a,twist:Ta(r.twist,n.twist,a)}}}const Vs=["rectangle","circle","line","triangle","diamond","pentagon","hexagon","star","heart","arrow"];function ce(e){return Vs.includes(e)}function Jt(e,t){t&&e.fill(),e.stroke()}function Wc(e,t,s,i,r,n){const a=s+r/2,o=i+n/2,h=r/2,c=n/2;for(let l=0;l<t;l++){const d=-Math.PI/2+l*Math.PI*2/t,u=a+Math.cos(d)*h,_=o+Math.sin(d)*c;l===0?e.moveTo(u,_):e.lineTo(u,_)}e.closePath()}function Yc(e,t,s){const i=s/2+2,r=Math.min(e.x,t.x)-i,n=Math.min(e.y,t.y)-i;return{x:r,y:n,w:Math.abs(t.x-e.x)+i*2,h:Math.abs(t.y-e.y)+i*2}}function Kr(e,t,s,i,r,n,a,o){const h=Math.min(s.x,i.x),c=Math.min(s.y,i.y),l=Math.abs(i.x-s.x),d=Math.abs(i.y-s.y);switch(e.save(),e.strokeStyle=r,e.lineWidth=o,e.lineCap="round",e.lineJoin="round",a&&(e.fillStyle=n),t){case"line":e.beginPath(),e.moveTo(s.x,s.y),e.lineTo(i.x,i.y),e.stroke();break;case"rectangle":{e.beginPath(),e.rect(h,c,l,d),Jt(e,a);break}case"circle":{const u=h+l/2,_=c+d/2;e.beginPath(),e.ellipse(u,_,l/2,d/2,0,0,Math.PI*2),Jt(e,a);break}case"triangle":{e.beginPath(),e.moveTo(h+l/2,c),e.lineTo(h+l,c+d),e.lineTo(h,c+d),e.closePath(),Jt(e,a);break}case"diamond":{e.beginPath(),e.moveTo(h+l/2,c),e.lineTo(h+l,c+d/2),e.lineTo(h+l/2,c+d),e.lineTo(h,c+d/2),e.closePath(),Jt(e,a);break}case"pentagon":{e.beginPath(),Wc(e,5,h,c,l,d),Jt(e,a);break}case"hexagon":{e.beginPath(),e.moveTo(h+l*.25,c),e.lineTo(h+l*.75,c),e.lineTo(h+l,c+d/2),e.lineTo(h+l*.75,c+d),e.lineTo(h+l*.25,c+d),e.lineTo(h,c+d/2),e.closePath(),Jt(e,a);break}case"star":{const u=h+l/2,_=c+d/2,f=l/2,m=d/2,p=.42;e.beginPath();for(let g=0;g<10;g++){const y=-Math.PI/2+g*Math.PI/5,w=g%2===0?1:p,x=u+Math.cos(y)*f*w,b=_+Math.sin(y)*m*w;g===0?e.moveTo(x,b):e.lineTo(x,b)}e.closePath(),Jt(e,a);break}case"heart":{e.beginPath(),e.moveTo(h+l/2,c+d),e.bezierCurveTo(h+l*.12,c+d*.72,h,c+d*.42,h,c+d*.27),e.bezierCurveTo(h,c+d*.04,h+l*.38,c,h+l/2,c+d*.2),e.bezierCurveTo(h+l*.62,c,h+l,c+d*.04,h+l,c+d*.27),e.bezierCurveTo(h+l,c+d*.42,h+l*.88,c+d*.72,h+l/2,c+d),e.closePath(),Jt(e,a);break}case"arrow":{e.beginPath(),e.moveTo(h,c+d*.3),e.lineTo(h+l*.58,c+d*.3),e.lineTo(h+l*.58,c),e.lineTo(h+l,c+d/2),e.lineTo(h+l*.58,c+d),e.lineTo(h+l*.58,c+d*.7),e.lineTo(h,c+d*.7),e.closePath(),Jt(e,a);break}}e.restore()}/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const Uc={CHILD:2},Vc=e=>(...t)=>({_$litDirective$:e,values:t});let Gc=class{constructor(t){}get _$AU(){return this._$AM._$AU}_$AT(t,s,i){this._$Ct=t,this._$AM=s,this._$Ci=i}_$AS(t,s){return this.update(t,s)}update(t,s){return this.render(...s)}};/**
 * @license
 * Copyright 2020 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const{I:qc}=mo,Zr=e=>e,Qr=()=>document.createComment(""),Ye=(e,t,s)=>{const i=e._$AA.parentNode,r=t===void 0?e._$AB:t._$AA;if(s===void 0){const n=i.insertBefore(Qr(),r),a=i.insertBefore(Qr(),r);s=new qc(n,a,e,e.options)}else{const n=s._$AB.nextSibling,a=s._$AM,o=a!==e;if(o){let h;s._$AQ?.(e),s._$AM=e,s._$AP!==void 0&&(h=e._$AU)!==a._$AU&&s._$AP(h)}if(n!==r||o){let h=s._$AA;for(;h!==n;){const c=Zr(h).nextSibling;Zr(i).insertBefore(h,r),h=c}}}return s},fe=(e,t,s=e)=>(e._$AI(t,s),e),Kc={},Zc=(e,t=Kc)=>e._$AH=t,Qc=e=>e._$AH,hi=e=>{e._$AR(),e._$AA.remove()};/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const Jr=(e,t,s)=>{const i=new Map;for(let r=t;r<=s;r++)i.set(e[r],r);return i},$a=Vc(class extends Gc{constructor(e){if(super(e),e.type!==Uc.CHILD)throw Error("repeat() can only be used in text expressions")}dt(e,t,s){let i;s===void 0?s=t:t!==void 0&&(i=t);const r=[],n=[];let a=0;for(const o of e)r[a]=i?i(o,a):a,n[a]=s(o,a),a++;return{values:n,keys:r}}render(e,t,s){return this.dt(e,t,s).values}update(e,[t,s,i]){const r=Qc(e),{values:n,keys:a}=this.dt(t,s,i);if(!Array.isArray(r))return this.ut=a,n;const o=this.ut??=[],h=[];let c,l,d=0,u=r.length-1,_=0,f=n.length-1;for(;d<=u&&_<=f;)if(r[d]===null)d++;else if(r[u]===null)u--;else if(o[d]===a[_])h[_]=fe(r[d],n[_]),d++,_++;else if(o[u]===a[f])h[f]=fe(r[u],n[f]),u--,f--;else if(o[d]===a[f])h[f]=fe(r[d],n[f]),Ye(e,h[f+1],r[d]),d++,f--;else if(o[u]===a[_])h[_]=fe(r[u],n[_]),Ye(e,r[d],r[u]),u--,_++;else if(c===void 0&&(c=Jr(a,_,f),l=Jr(o,d,u)),c.has(o[d]))if(c.has(o[u])){const m=l.get(a[_]),p=m!==void 0?r[m]:null;if(p===null){const g=Ye(e,r[d]);fe(g,n[_]),h[_]=g}else h[_]=fe(p,n[_]),Ye(e,r[d],p),r[m]=null;_++}else hi(r[u]),u--;else hi(r[d]),d++;for(;_<=f;){const m=Ye(e,h[f+1]);fe(m,n[_]),h[_++]=m}for(;d<=u;){const m=r[d++];m!==null&&hi(m)}return this.ut=a,Zc(e,h),Pe}}),tn=Q("fan"),en=Q("splatter");function Jc(e,t,s){const i=Math.max(1,e),r=kt(i,i),n=ot(r),a=i/2;if(t>=1)n.fillStyle="#fff",n.beginPath(),n.arc(a,a,a,0,Math.PI*2),n.fill();else{const o=n.createRadialGradient(a,a,a*t,a,a,a);o.addColorStop(0,"rgba(255,255,255,1)"),o.addColorStop(1,"rgba(255,255,255,0)"),n.fillStyle=o,n.beginPath(),n.arc(a,a,a,0,Math.PI*2),n.fill()}return r}function tl(e,t,s){const i=Math.max(1,e),r=Math.max(1,Math.ceil(e/Math.max(1,s.aspect))),n=kt(i,r),a=ot(n);if(t>=1)a.fillStyle="#fff",a.fillRect(0,0,i,r);else{a.fillStyle="#fff",a.fillRect(0,0,i,r);const o=Math.max(1,(1-t)*Math.min(i,r)*.5);a.globalCompositeOperation="destination-in";const h=a.createLinearGradient(0,0,i,0);h.addColorStop(0,"rgba(255,255,255,0)"),h.addColorStop(Math.min(.5,o/i),"rgba(255,255,255,1)"),h.addColorStop(Math.max(.5,1-o/i),"rgba(255,255,255,1)"),h.addColorStop(1,"rgba(255,255,255,0)"),a.fillStyle=h,a.fillRect(0,0,i,r);const c=a.createLinearGradient(0,0,0,r);c.addColorStop(0,"rgba(255,255,255,0)"),c.addColorStop(Math.min(.5,o/r),"rgba(255,255,255,1)"),c.addColorStop(Math.max(.5,1-o/r),"rgba(255,255,255,1)"),c.addColorStop(1,"rgba(255,255,255,0)"),a.fillStyle=c,a.fillRect(0,0,i,r),a.globalCompositeOperation="source-over"}return n}function el(e,t,s){const i=Math.max(1,e),r=Math.max(1,Math.ceil(e/Math.max(1,s.aspect))),n=kt(i,r),a=ot(n),o=i/Math.max(1,s.aspect)/3;if(a.beginPath(),a.moveTo(o,0),a.lineTo(i,0),a.lineTo(i-o,r),a.lineTo(0,r),a.closePath(),t>=1)a.fillStyle="#fff",a.fill();else{a.fillStyle="#fff",a.fill();const h=i/2,c=r/2,l=Math.sqrt(h*h+c*c);a.globalCompositeOperation="destination-in";const d=a.createRadialGradient(h,c,l*t,h,c,l);d.addColorStop(0,"rgba(255,255,255,1)"),d.addColorStop(1,"rgba(255,255,255,0)"),a.fillStyle=d,a.fillRect(0,0,i,r),a.globalCompositeOperation="source-over"}return n}function sl(e,t,s){const i=Math.max(1,e),r=Math.max(2,Math.ceil(e/Math.max(1,s.aspect))),n=i/2,a=r/2,o=kt(i,r),h=ot(o),c=i/2,l=r/2;if(t>=1)h.fillStyle="#fff",h.beginPath(),h.ellipse(c,l,n,a,0,0,Math.PI*2),h.fill();else{h.save(),h.translate(c,l),h.scale(1,a/n);const d=h.createRadialGradient(0,0,n*t,0,0,n);d.addColorStop(0,"rgba(255,255,255,1)"),d.addColorStop(1,"rgba(255,255,255,0)"),h.fillStyle=d,h.beginPath(),h.arc(0,0,n,0,Math.PI*2),h.fill(),h.restore()}return o}function Ia(e){let t=e|0;return()=>{t=t+1831565813|0;let s=Math.imul(t^t>>>15,1|t);return s=s+Math.imul(s^s>>>7,61|s)^s,((s^s>>>14)>>>0)/4294967296}}function Ea(e,t,s,i=0){const r=s.bristles??tn.bristles,a=(s.spread??tn.spread)*Math.PI/180,o=e/2,h=Math.max(1,e/8),c=kt(e,e),l=ot(c),d=e/2,u=e/2,_=-Math.PI/2-a/2,f=Ia(42+i*7);for(let m=0;m<r;m++){const p=r>1?m/(r-1):.5,g=_+a*p,y=o*.08*(f()-.5),w=.05*(f()-.5),x=d+Math.cos(g+w)*(o-h+y),b=u+Math.sin(g+w)*(o-h+y);if(t>=1)l.fillStyle="#fff",l.beginPath(),l.arc(x,b,h,0,Math.PI*2),l.fill();else{const S=l.createRadialGradient(x,b,h*t,x,b,h);S.addColorStop(0,"rgba(255,255,255,1)"),S.addColorStop(1,"rgba(255,255,255,0)"),l.fillStyle=S,l.beginPath(),l.arc(x,b,h,0,Math.PI*2),l.fill()}}return c}function Ra(e,t,s,i=0){const r=s.bristles??en.bristles,n=s.spread??en.spread,a=e/2*n,o=Math.max(1,e/10),h=kt(e,e),c=ot(h),l=e/2,d=e/2,u=Ia(137+i*13);for(let _=0;_<r;_++){const f=u()*Math.PI*2,m=u()*a,p=l+Math.cos(f)*m,g=d+Math.sin(f)*m,y=o*(.5+u()*.5);if(t>=1)c.fillStyle="#fff",c.beginPath(),c.arc(p,g,y,0,Math.PI*2),c.fill();else{const w=c.createRadialGradient(p,g,y*t,p,g,y);w.addColorStop(0,"rgba(255,255,255,1)"),w.addColorStop(1,"rgba(255,255,255,0)"),c.fillStyle=w,c.beginPath(),c.arc(p,g,y,0,Math.PI*2),c.fill()}}return h}const ji={round:Jc,flat:tl,chisel:el,calligraphy:sl,fan:Ea,splatter:Ra,image:hh};function zi(e,t){const s=Math.max(1,e),i=Math.max(1,t.aspect);switch(t.shape){case"flat":case"chisel":return{w:s,h:Math.max(1,e/i)};case"calligraphy":return{w:s,h:Math.max(2,e/i)};case"image":return da(e,t);default:return{w:s,h:s}}}const il={fan:4,splatter:6},rl=32*1024*1024;class nl{constructor(){this._entries=new Map,this._bytes=0}_buildKey(t,s,i,r){let n=i.shape==="image"?`image-${i.imageId}-r${xs(i.imageId)}-${t}`:`${i.shape}-${t}-${s.toFixed(2)}-${i.aspect.toFixed(1)}`;return i.bristles!=null&&(n+=`-b${i.bristles}`),i.spread!=null&&(n+=`-s${i.spread.toFixed(2)}`),r!=null&&(n+=`-v${r}`),n}get(t,s,i){const r=this._buildKey(t,s,i),n=this._lookup(r);if(n)return n;const a=ji[i.shape];return this._insert(r,a(t,s,i))}getVariant(t,s,i,r){const n=this._buildKey(t,s,i,r),a=this._lookup(n);if(a)return a;let o;return i.shape==="fan"?o=Ea(t,s,i,r):i.shape==="splatter"?o=Ra(t,s,i,r):o=ji[i.shape](t,s,i),this._insert(n,o)}_lookup(t){const s=this._entries.get(t);return s&&(this._entries.delete(t),this._entries.set(t,s)),s}_insert(t,s){this._entries.set(t,s),this._bytes+=s.width*s.height*4;for(const[i,r]of this._entries){if(this._bytes<=rl||i===t)break;this._entries.delete(i),this._bytes-=r.width*r.height*4}return s}clear(){this._entries.clear(),this._bytes=0}}const Ae=(e,t,s)=>.3*e+.59*t+.11*s,sn=(e,t,s)=>Math.max(e,t,s)-Math.min(e,t,s),ie=new Float64Array(3);function fs(e,t,s,i,r){const n=i-Ae(e,t,s);e+=n,t+=n,s+=n;const a=Ae(e,t,s),o=Math.min(e,t,s),h=Math.max(e,t,s);if(o<0){const c=a/(a-o);e=a+(e-a)*c,t=a+(t-a)*c,s=a+(s-a)*c}if(h>1){const c=(1-a)/(h-a);e=a+(e-a)*c,t=a+(t-a)*c,s=a+(s-a)*c}r[0]=e,r[1]=t,r[2]=s}function rn(e,t,s,i,r){const n=Math.min(e,t,s),a=Math.max(e,t,s);if(a>n){const o=i/(a-n);r[0]=(e-n)*o,r[1]=(t-n)*o,r[2]=(s-n)*o}else r[0]=0,r[1]=0,r[2]=0}function ci(e,t,s){switch(e){case"multiply":return t*s;case"screen":return t+s-t*s;case"overlay":return t<=.5?2*t*s:1-2*(1-t)*(1-s);case"darken":return Math.min(t,s);case"lighten":return Math.max(t,s);case"color-dodge":return t===0?0:s>=1?1:Math.min(1,t/(1-s));case"color-burn":return t>=1?1:s<=0?0:1-Math.min(1,(1-t)/s);default:return s}}function al(e,t,s,i,r,n,a,o){switch(e){case"hue":rn(r,n,a,sn(t,s,i),ie),fs(ie[0],ie[1],ie[2],Ae(t,s,i),o);return;case"saturation":rn(t,s,i,sn(r,n,a),ie),fs(ie[0],ie[1],ie[2],Ae(t,s,i),o);return;case"color":fs(r,n,a,Ae(t,s,i),o);return;case"luminosity":fs(t,s,i,Ae(r,n,a),o);return;default:o[0]=ci(e,t,r),o[1]=ci(e,s,n),o[2]=ci(e,i,a)}}function ol(e,t,s,i){const r=e.data,n=t.data,a=new Float64Array(3);for(let o=0;o<r.length;o+=4){const h=n[o+3]/255*s;if(h<=0||r[o+3]===0)continue;const c=r[o]/255,l=r[o+1]/255,d=r[o+2]/255;al(i,c,l,d,n[o]/255,n[o+1]/255,n[o+2]/255,a),r[o]=Math.round(255*(h*a[0]+(1-h)*c)),r[o+1]=Math.round(255*(h*a[1]+(1-h)*l)),r[o+2]=Math.round(255*(h*a[2]+(1-h)*d))}}function Da(e,t,s){return e||t==="clear"?s?null:"destination-out":t==="behind"?s?null:"destination-over":t==="normal"?s?"source-atop":"source-over":t}function Aa(e){const t=Da(e.eraser,e.mode,e.alphaLocked);return e.alphaLocked&&t!==null&&t!=="source-atop"}function La(e,t,s,i){const r=Da(i.eraser,i.mode,i.alphaLocked);if(!r)return;const{x:n,y:a,w:o,h}=s,c=t;if(Aa(i)){const l=e.getImageData(n,a,o,h);ol(l,ot(t).getImageData(n,a,o,h),i.opacity,i.mode),e.putImageData(l,n,a);return}e.save(),e.globalAlpha=i.opacity,e.globalCompositeOperation=r,e.drawImage(c,n,a,o,h,n,a,o,h),e.restore()}class hl{constructor(){this._canvas=null,this._width=0,this._height=0}acquire(t,s,i){return!this._canvas||t>this._width||s>this._height?(this._width=Math.max(this._width,t),this._height=Math.max(this._height,s),this._canvas=kt(this._width,this._height),this._canvas):(i&&ot(this._canvas).clearRect(i.x,i.y,i.w,i.h),this._canvas)}commit(t,s,i,r,n,a=!1,o="normal",h=!1,c=null){const l=c??this._canvas;if(!l||!n)return;const{x:d,y:u,w:_,h:f}=n;if(!r&&!a){const m=ot(l);m.save(),m.beginPath(),m.rect(d,u,_,f),m.clip(),m.globalCompositeOperation="source-in",m.fillStyle=s,m.fillRect(d,u,_,f),m.restore()}La(t,l,n,{opacity:i,eraser:r,mode:o,alphaLocked:h})}get current(){return this._canvas}}function nn(e,t,s){return{velocity:e.velocity+(t.velocity-e.velocity)*s,tiltX:e.tiltX+(t.tiltX-e.tiltX)*s,tiltY:e.tiltY+(t.tiltY-e.tiltY)*s,twist:Ta(e.twist,t.twist,s)}}const li=1,cl=.001,ll=.25,dl=1,ul=16;function an(e,t,s){return Math.max(cl,typeof e=="number"?e:e(t,s))}class pl{constructor(){this._window=[],this._count=0,this._last=null,this._lastSpacing=0,this._heading=null,this._since=0,this._lastDist=0,this._tailLaid=!1,this._laid=0}reset(){this._laid=0,this._window=[],this._count=0,this._last=null,this._lastSpacing=0,this._heading=null,this._since=0,this._lastDist=0,this._tailLaid=!1}laidCount(){return this._laid}pathLength(){return this._lastDist+this._since}addPoint(t,s,i,r,n=0,a={}){const o={x:t,y:s,pressure:i,timestamp:n,velocity:a.velocity??0,tiltX:a.tiltX??0,tiltY:a.tiltY??0,twist:a.twist??0};this._count++,this._window.push(o),this._window.length>4&&this._window.shift();const h=this._count,c=this._tailLaid;if(this._tailLaid=!1,h===1){const f={x:t,y:s,pressure:i,speedPxPerMs:li,pathDist:0,dab:0,velocity:o.velocity,tiltX:o.tiltX,tiltY:o.tiltY,twist:o.twist};return this._lay(f,r),[f]}if(h===2)return this._walkLinear(this._window[0],this._window[1],r);if(h===3)return[];if(c)return[];const[l,d,u,_]=this._window;return this._walkCatmullRom(l,d,u,_,r)}settle(t){if(this._tailLaid)return[];const s=this.flush(t);return this._count>=3&&(this._tailLaid=!0),s}flush(t){if(this._tailLaid)return[];const s=this._count,i=this._window;if(s<2)return[];if(s===2)return[];if(s===3)return this._walkLinear(i[i.length-2],i[i.length-1],t);const r=i[1],n=i[2],a=i[3],o={x:a.x+(a.x-n.x),y:a.y+(a.y-n.y),pressure:a.pressure,timestamp:a.timestamp+Math.max(0,a.timestamp-n.timestamp),velocity:a.velocity,tiltX:a.tiltX,tiltY:a.tiltY,twist:a.twist};return this._walkCatmullRom(r,n,a,o,t)}_walkLinear(t,s,i){const r=s.x-t.x,n=s.y-t.y,a=Math.sqrt(r*r+n*n);if(a<.001)return[];const o=s.timestamp-t.timestamp,h=o>0?a/o:li;return this._walk(a,i,c=>{const l=c/a;return{x:t.x+r*l,y:t.y+n*l,pressure:t.pressure+(s.pressure-t.pressure)*l,speedPxPerMs:h,pathDist:0,dab:0,...nn(t,s,l)}})}_walkCatmullRom(t,s,i,r,n){let o=0,h=s.x,c=s.y;const l=[0];for(let _=1;_<=20;_++){const f=_/20,m=_s(t.x,s.x,i.x,r.x,f),p=_s(t.y,s.y,i.y,r.y,f),g=Math.sqrt((m-h)**2+(p-c)**2);o+=g,l.push(o),h=m,c=p}if(o<.001)return[];const d=i.timestamp-s.timestamp,u=d>0?o/d:li;return this._walk(o,n,_=>{let f=19;for(let w=1;w<l.length;w++)if(l[w]>=_){f=w-1;break}const m=l[f],p=l[f+1],g=p>m?Math.min(1,(_-m)/(p-m)):0,y=(f+g)/20;return{x:_s(t.x,s.x,i.x,r.x,y),y:_s(t.y,s.y,i.y,r.y,y),pressure:s.pressure+(i.pressure-s.pressure)*y,speedPxPerMs:u,pathDist:0,dab:0,...nn(s,i,y)}})}_walk(t,s,i){const r=[];let n=0;for(;;){const a=this._since,o=this._lastSpacing-a,h=n+o;let c=-1;if(typeof s=="number")h<=t&&(c=h);else{const d=_=>{const f=i(_);return f.pathDist=this._lastDist+a+(_-n),f.dab=this._laid,a+(_-n)>=Math.min(this._lastSpacing,an(s,f,this._headingTo(f)))},u=Math.min(t,h);if(a>0&&d(n))c=n;else if(u>n){const _=Math.ceil((u-n)/dl);let f=n;for(let m=1;m<=_;m++){const p=m===_?u:n+(u-n)*m/_;if(p>=h||d(p)){let g=p;for(let y=0;y<ul;y++){const w=(f+g)/2;d(w)?g=w:f=w}c=g;break}f=p}}}if(c<0)return this._since=a+(t-n),r;const l=i(c);l.pathDist=this._lastDist+a+(c-n),l.dab=this._laid,this._lay(l,s),r.push(l),n=c}}_headingTo(t){const s=this._last;if(!s)return null;const i=t.x-s.x,r=t.y-s.y;return i*i+r*r>=ll?Math.atan2(r,i):this._heading}_lay(t,s){this._heading=this._headingTo(t),this._last=t,this._lastDist=t.pathDist,this._laid++,this._lastSpacing=an(s,t,this._heading),this._since=0}}function _s(e,t,s,i,r){const n=r*r,a=n*r;return .5*(2*t+(-e+s)*r+(2*e-5*t+4*s-i)*n+(-e+3*t-3*s+i)*a)}function di(e){return e<=.04045?e/12.92:Math.pow((e+.055)/1.055,2.4)}function ui(e){return e<=.0031308?e*12.92:1.055*Math.pow(e,1/2.4)-.055}function Oi(e,t,s){const i=di(e),r=di(t),n=di(s),a=Math.cbrt(.4122214708*i+.5363325363*r+.0514459929*n),o=Math.cbrt(.2119034982*i+.6806995451*r+.1073969566*n),h=Math.cbrt(.0883024619*i+.2817188376*r+.6299787005*n);return{L:.2104542553*a+.793617785*o-.0040720468*h,a:1.9779984951*a-2.428592205*o+.4505937099*h,b:.0259040371*a+.7827717662*o-.808675766*h}}function ja(e,t,s){const i=e+.3963377774*t+.2158037573*s,r=e-.1055613458*t-.0638541728*s,n=e-.0894841775*t-1.291485548*s,a=i*i*i,o=r*r*r,h=n*n*n;return{r:Math.max(0,Math.min(1,ui(4.0767416621*a-3.3077115913*o+.2309699292*h))),g:Math.max(0,Math.min(1,ui(-1.2684380046*a+2.6097574011*o-.3413193965*h))),b:Math.max(0,Math.min(1,ui(-.0041960863*a-.7034186147*o+1.707614701*h)))}}function on(e,t,s){const i=e+.3963377774*t+.2158037573*s,r=e-.1055613458*t-.0638541728*s,n=e-.0894841775*t-1.291485548*s,a=i*i*i,o=r*r*r,h=n*n*n,c=1e-4,l=4.0767416621*a-3.3077115913*o+.2309699292*h,d=-1.2684380046*a+2.6097574011*o-.3413193965*h,u=-.0041960863*a-.7034186147*o+1.707614701*h;return l>=-c&&l<=1+c&&d>=-c&&d<=1+c&&u>=-c&&u<=1+c}function fl(e,t,s){const i=Math.cos(s*2*Math.PI),r=Math.sin(s*2*Math.PI);if(on(e,i*t,r*t))return t;let n=0,a=t;for(let o=0;o<10;o++){const h=(n+a)/2;on(e,i*h,r*h)?n=h:a=h}return n}function Bi(e){const t=parseInt(e.slice(1,7),16);return{r:(t>>16&255)/255,g:(t>>8&255)/255,b:(t&255)/255}}function za(e){const t=Math.round(e.r*255),s=Math.round(e.g*255),i=Math.round(e.b*255);return`#${(1<<24|t<<16|s<<8|i).toString(16).slice(1)}`}function Oa(e,t,s){if(s<=0)return e;if(s>=1)return t;const i=Bi(e),r=Bi(t),n=Oi(i.r,i.g,i.b),a=Oi(r.r,r.g,r.b),o=ja(n.L+(a.L-n.L)*s,n.a+(a.a-n.a)*s,n.b+(a.b-n.b)*s);return za(o)}function _l(e,t){return{distanceTraveled:0,remainingPaint:1,originalColor:e,currentColor:e,stampCount:0,layerSnapshot:t,prevRotation:0,pickedColor:e,pickedWeight:0}}function ml(e,t){if(e.depletion<=0)return 1;const s=Math.max(0,1-t.distanceTraveled/Math.max(1,e.depletionLength)*e.depletion);return t.remainingPaint=s,s}const gl=1;function yl(e,t,s){if(e.buildup<=0)return t;const r=1-Math.min(1,Math.max(0,s/gl));return Math.min(1,t*(1+e.buildup*r*3))}const vl=48;function bl(e,t,s,i=6){const r=e.width,n=e.height,a=e.data,o=Math.round(t),h=Math.round(s),c=Math.max(1,Math.round(i)),l=c*c,d=Math.max(1,Math.ceil((2*c+1)/vl));let u=0,_=0,f=0,m=0,p=0;const g=Math.max(0,o-c),y=Math.min(r-1,o+c),w=Math.max(0,h-c),x=Math.min(n-1,h+c);for(let L=w;L<=x;L+=d){const I=L-h;for(let k=g;k<=y;k+=d){const M=k-o;if(M*M+I*I>l)continue;const P=(L*r+k)*4,R=a[P+3];R<2||(u+=a[P]*R,_+=a[P+1]*R,f+=a[P+2]*R,m+=R,p++)}}if(p===0||m===0)return{color:"#000000",alpha:0};const b=Math.round(u/m),S=Math.round(_/m),E=Math.round(f/m),D=Math.round(m/p);return{color:`#${(1<<24|b<<16|S<<8|E).toString(16).slice(1)}`,alpha:D}}function hn(e,t,s,i,r=6){if(e.wetness<=0||!t.layerSnapshot)return;const n=bl(t.layerSnapshot,s,i,Math.max(4,r));if(n.alpha<10){t.pickedWeight=0,t.currentColor=t.originalColor;return}const a=n.alpha/255;t.pickedColor=n.color,t.pickedWeight=e.wetness*a,t.currentColor=Oa(t.originalColor,n.color,t.pickedWeight)}const cn=64,ln=32,dn=32,un=32;function pn(e){if(!/^#[0-9a-f]{6}$/i.test(e))return null;const t=Bi(e),s=Oi(t.r,t.g,t.b),i=Math.atan2(s.b,s.a)/(2*Math.PI);return[s.L,Math.hypot(s.a,s.b),i-Math.floor(i)]}function wl(e,t,s,i,r){const n=pn(e);if(!n)return e;let[a,o,h]=n,c=e;if(s.fgBg>0){const y=pn(t);if(y){const w=s.fgBg*(s.fgBgPressure?1-Math.max(0,Math.min(1,r)):i.fgBg),x=Math.round(Math.max(0,Math.min(1,w))*un)/un;if(x>=1)c=t,[a,o,h]=y;else if(x>0){c=null;const b=Math.cos(h*2*Math.PI)*o,S=Math.sin(h*2*Math.PI)*o,E=Math.cos(y[2]*2*Math.PI)*y[1],D=Math.sin(y[2]*2*Math.PI)*y[1],L=b+(E-b)*x,I=S+(D-S)*x;a+=(y[0]-a)*x,o=Math.hypot(L,I),h=Math.atan2(I,L)/(2*Math.PI),h-=Math.floor(h)}}}let l=(i.hue*2-1)*s.hue*.5;l=Math.round(l*cn)/cn;const d=Math.max(0,(1+s.purity)*(1+(i.saturation*2-1)*s.saturation)),u=Math.round(d*dn)/dn,_=a+(i.brightness*2-1)*s.brightness*.5,f=_<=0?0:_>=1?1:a+Math.round((_-a)*ln)/ln;if(c!==null&&l===0&&u===1&&f===a)return c;const m=h+l,p=Math.max(0,Math.min(1,f)),g=p<=0||p>=1?0:fl(p,Math.min(.5,o*u),m);return za(ja(p,Math.cos(m*2*Math.PI)*g,Math.sin(m*2*Math.PI)*g))}const xl=2654435769,kl=2246822507,Ml=3266489909,Cl=16*1024*1024,ms=32,re=2,Sl=128,Ba=.25,Pl=40,Tl=2,$l=2,fn=89.9,Il=1,_n=250,El=32,Rl=.25,Dl=.01,Al=64,Ll={tiltX:0,tiltY:0};function Be(e){return Number.isFinite(e)?Math.max(-fn,Math.min(fn,e)):0}function jl(e){return e!==void 0&&Number.isFinite(e)?Ai(e):0}const zl=.5;function Ol(e,t){const s=Be(e),i=Be(t);return Math.hypot(s,i)<zl?null:Math.atan2(Math.tan(i*Math.PI/180),Math.tan(s*Math.PI/180))}const Ha={azimuth:0,stretch:1};function Hi(e,t,s){if(!(s>0)||e===0&&t===0)return Ha;const i=Math.tan(Be(e)*Math.PI/180),r=Math.tan(Be(t)*Math.PI/180),n=Math.atan(Math.hypot(i,r))/(Math.PI/2);return{azimuth:Math.atan2(r,i),stretch:1+s*$l*n}}function mn(e,t,s,i,r,n=1){return i===0&&r.stretch===1&&n===1?!1:(e.save(),e.translate(t,s),r.stretch!==1?(e.rotate(r.azimuth),e.scale(r.stretch,1),e.rotate(i-r.azimuth)):e.rotate(i),n!==1&&e.scale(1,n),!0)}const Bl=new Set(["flat","chisel","calligraphy"]),Hl=Array.from({length:8},(e,t)=>t*Math.PI/8);function Fl(e,t,s,i){const r=Math.abs(i)*t;return e==="calligraphy"?Math.hypot(s,r):e==="chisel"?Math.max(r,Math.abs(s+i/3)/(1-1/(3*t))):Math.max(Math.abs(s),r)}function gn(e,t,s,i,r){const n=r-i.azimuth,a=Math.cos(n)/i.stretch,o=Math.sin(n),h=i.azimuth-s,c=a*Math.cos(h)-o*Math.sin(h),l=a*Math.sin(h)+o*Math.cos(h);return 1/Math.max(Fl(e,t,c,l),1e-9)}function yn(e,t){return e>=0?1-e+e*t:1+e*t}function Fa(e,t){const s=t.angle*Math.PI/180;if(t.orientation==="twist")return e.twist*Math.PI/180+s;if(t.orientation==="tilt"){const i=Ol(e.tiltX,e.tiltY);return i===null?null:i+s}return null}function Nl(e,t,s,i){if(i.orientation==="fixed")return i.angle*Math.PI/180;if(i.orientation==="tilt"||i.orientation==="twist")return Fa(e,i)??(t?s:i.angle*Math.PI/180);if(!t)return i.angle*Math.PI/180;const r=e.x-t.x,n=e.y-t.y;return r*r+n*n<Ba?s:Math.atan2(n,r)+i.angle*Math.PI/180}class Na{constructor(){this._tipCache=new nl,this._bufferPool=new hl,this._smoother=new pl,this._stabilizer=new Xc,this._viewScale=1,this._velocity=0,this._velocityLast=null,this._velocityDist=0,this._descriptor=null,this._color="",this._eraser=!1,this._alphaLocked=!1,this._colorMode=!1,this._secondary="",this._dynamicColor=!1,this._colorRng=new te(0),this._strokeRolls=null,this._strokeDabColor=null,this._docWidth=0,this._docHeight=0,this._inkState=null,this._prevStamp=null,this._unpinTip=()=>{},this._distBase=0,this._dabBase=0,this._variantCounter=0,this._rng=new te(0),this._flowRng=new te(0),this._scatterRng=new te(0),this._heading=null,this._airClock=null,this._tailPending=!1,this._lastFeedStep=0,this._lastEvent=0,this._snapshotCaptured=!1,this._grain=null,this._grainCanvas=null,this._grainDeltaMinX=1/0,this._grainDeltaMinY=1/0,this._grainDeltaMaxX=-1/0,this._grainDeltaMaxY=-1/0,this._dabScratch=null,this._tintCanvas=null,this._tintW=0,this._tintH=0,this._tintedTip=null,this._tintedColor="",this._tintCache=new Map,this._slotW=0,this._slotH=0,this._tipIds=new WeakMap,this._nextTipId=1,this._dirtyMinX=1/0,this._dirtyMinY=1/0,this._dirtyMaxX=-1/0,this._dirtyMaxY=-1/0,this._deltaMinX=1/0,this._deltaMinY=1/0,this._deltaMaxX=-1/0,this._deltaMaxY=-1/0,this._dabSpacing=(t,s)=>{const i=this._descriptor;return Math.max(1,i.spacing*this._sizeAt(t)*this._leanSpacing(t,s))}}begin(t,s,i,r,n,a={}){this._descriptor=Oe(t),this._descriptor.tip=ah(this._descriptor.tip),this._unpinTip(),this._descriptor.tip.shape==="image"&&(this._unpinTip=ih(this._descriptor.tip.imageId)),this._viewScale=a.viewScale&&a.viewScale>0?a.viewScale:1,this._resetMotion(),this._color=i?"":s.length===9?s.slice(0,7):s,this._eraser=i,this._alphaLocked=a.alphaLocked===!0,this._secondary=a.secondaryColor&&a.secondaryColor.length===9?a.secondaryColor.slice(0,7):a.secondaryColor??"",this._dynamicColor=!i&&rr(t.color),this._colorMode=!i&&(t.ink.wetness>0||this._dynamicColor),this._docWidth=r,this._docHeight=n;const o=this._rawDirtyRect();this._bufferPool.acquire(r,n,o),this._grainCanvas&&o&&ot(this._grainCanvas).clearRect(o.x,o.y,o.w,o.h),this._grain=Lo(this._descriptor.texture),this._resetGrainDelta(),this._smoother.reset(),this._stabilizer.reset(a.stabilizerMs??0,Il/this._viewScale),this._prevStamp=null,this._distBase=0,this._dabBase=0,this._heading=null,this._airClock=null,this._lastFeedStep=0,this._tailPending=!1,this._variantCounter=0;const h=a.seed??Ro();this._rng=new te(h),this._flowRng=new te(h^xl),this._scatterRng=new te(h^kl),this._colorRng=new te(h^Ml),this._strokeRolls=null,this._strokeDabColor=null,this._clearTintCache(),this._snapshotCaptured=!1,this._dirtyMinX=1/0,this._dirtyMinY=1/0,this._dirtyMaxX=-1/0,this._dirtyMaxY=-1/0,this._resetPreviewDelta(),this._inkState=_l(this._color,null)}stroke(t,s,i,r,n=0,a=Ll){if(!this._descriptor||!this._inkState)return;const o=this._descriptor;let h=i;if(!Number.isFinite(i)||i<=0){if(o.pressureSize||o.pressureOpacity||o.fade.taperStart>0||this._dynamicColor&&o.color.fgBgPressure&&o.color.fgBg>0||this._grain?.perTip&&this._grain.depthPressure){this._finishSegment(),this._distBase+=this._smoother.pathLength(),this._dabBase+=this._smoother.laidCount(),this._smoother.reset(),this._stabilizer.reset(),this._resetMotion(),this._prevStamp=null,this._heading=null,this._airClock=null,this._tailPending=!1;return}h=1}o.ink.wetness>0&&this._colorMode&&!this._snapshotCaptured&&r&&(this._inkState.layerSnapshot=r.getImageData(0,0,this._docWidth,this._docHeight),this._snapshotCaptured=!0);const c=Io[o.pressureCurve],l=this._stabilizer.add(t,s,c(h),n,Be(a.tiltX),Be(a.tiltY),jl(a.twist)),d=this._feed({...l,timestamp:n});this._stampPoints(d),o.airbrush&&(this._lastEvent=n,this._tailPending=!0,(d.length>0||this._airClock===null&&this._prevStamp)&&(this._airClock=n))}suspendAirbrush(){this._airClock=null}_settlePath(t,s){let i;if(s){const r=this._stabilizer.position(),n=this._stabilizer.penTrail(),a=this._velocityLast?Math.min(this._velocityLast.t,t):t;let o=this._stabilizer.hold(t);this._stabilizer.lag()<=Rl&&(o=this._stabilizer.snap(t));const h=Math.hypot(o.x-r.x,o.y-r.y),c=Math.max(1,this._lastFeedStep,this._prevStamp?this._dabSpacing(this._prevStamp,this._heading):1),l=Math.max(1,Math.min(Al,Math.ceil(h/c)));i=[];for(let d=1;d<=l;d++){const u=d/l;i.push(...this._feed({...o,...n(u),x:r.x+(o.x-r.x)*u,y:r.y+(o.y-r.y)*u,timestamp:a+(t-a)*u}))}if(l>1){const d=(l-1)/l;this._stabilizer.setPrevious(r.x+(o.x-r.x)*d,r.y+(o.y-r.y)*d,a+(t-a)*d)}}else i=this._smoother.settle(this._dabSpacing),this._tailPending=!1;return this._stampPoints(i),i.length}get airbrushActive(){return!!this._descriptor?.airbrush}tick(t){const s=this._descriptor;if(!s||!s.airbrush||!this._inkState||!this._prevStamp||this._airClock===null||!Number.isFinite(t)||!(t>this._airClock))return 0;const i=1e3/s.airbrushRate,r=t-this._lastEvent>=Math.max(El,i);if(r&&this._stabilizer.lag()>Dl)return this._airClock=t,this._settlePath(t,!0);t-this._airClock>_n&&(this._airClock=t-_n);const n=Math.floor((t-this._airClock)/i+1e-9);if(n<=0)return 0;if(r&&this._tailPending){const h=this._settlePath(t,!1);if(h>0)return this._airClock=t,h}const a=[],o=this._stabilizer.rawPen();for(let h=1;h<=n;h++){const c=this._airClock+h*i,l=this._velocityLast,d=l?this._trackVelocity(l.x,l.y,c):0;a.push({...this._prevStamp,...o,speedPxPerMs:0,velocity:d,dab:this._smoother.laidCount()+h-1})}return this._airClock+=n*i,this._stampPoints(a),this._dabBase+=n,n}_feed(t){this._velocityLast&&(this._lastFeedStep=Math.hypot(t.x-this._velocityLast.x,t.y-this._velocityLast.y));const s=this._trackVelocity(t.x,t.y,t.timestamp);return this._smoother.addPoint(t.x,t.y,t.pressure,this._dabSpacing,t.timestamp,{velocity:s,tiltX:t.tiltX,tiltY:t.tiltY,twist:t.twist})}_stampPoints(t){if(!this._descriptor||!this._inkState)return;const s=this._descriptor,i=s.ink,r=this._inkState,n=this._bufferPool.current;if(!n)return;const a=ot(n),o=il[s.tip.shape]??0,h=s.scatter,c=h.amount>0||h.count>1;for(const l of t){let d=0;if(this._prevStamp){const M=l.x-this._prevStamp.x,P=l.y-this._prevStamp.y;d=Math.sqrt(M*M+P*P),r.distanceTraveled+=d,M*M+P*P>=Ba&&(this._heading=Math.atan2(P,M))}r.stampCount++;const u=this._rng.next(),_=this._rng.next(),f=this._rng.next(),m=this._flowRng.next();let p=null;this._dynamicColor&&(s.color.perTip?p=this._rollColor():p=this._strokeRolls??=this._rollColor());const g=ml(i,r);if(g<=0){this._prevStamp=l;continue}const y=(s.pressureOpacity?s.flow*l.pressure:s.flow)*yn(s.dynamics.flowVelocity,l.velocity),w=yl(i,y,l.speedPxPerMs),x=this._sizeAt(l),b=Nl(l,this._prevStamp,r.prevRotation,s.tip);r.prevRotation=b;const S=Hi(l.tiltX,l.tiltY,s.dynamics.tilt);let E=1;if(c){const M=this._scatterRng.next();E=Math.max(1,Math.round(h.count*(1-h.countJitter*M)))}const D=h.amount*x,L=h.bothAxes||this._heading===null,I=this._heading===null?1:Math.cos(this._heading),k=this._heading===null?0:Math.sin(this._heading);for(let M=0;M<E;M++){let P=u,R=_,O=f,V=m,H=l.x,F=l.y;if(c){const it=(this._scatterRng.next()*2-1)*D,tt=(this._scatterRng.next()*2-1)*D;M>0&&(P=this._scatterRng.next(),R=this._scatterRng.next(),O=this._scatterRng.next(),V=this._scatterRng.next());const lt=L?tt:0;H+=I*lt-k*it,F+=k*lt+I*it}const nt=Math.max(1,x*(1-s.jitter.size*P));if(p){const it=M>0&&s.color.perTip?this._rollColor():p,tt=this._dabColor(it,l.pressure);r.originalColor=tt,r.currentColor=tt,M===0?hn(i,r,l.x,l.y,nt/2):r.pickedWeight>0&&(r.currentColor=Oa(tt,r.pickedColor,r.pickedWeight))}else M===0&&hn(i,r,l.x,l.y,nt/2);const vt=Eo(nt);let G;if(o>0){const it=this._variantCounter%o;G=this._tipCache.getVariant(vt,s.hardness,s.tip,it),this._variantCounter++}else G=this._tipCache.get(vt,s.hardness,s.tip);const Ht=b+(R-.5)*s.jitter.angle*2*Math.PI,Xt=1-s.jitter.roundness*O,Wt=Math.min(1,w*g)*(1-s.jitter.flow*V),Mt=G.width,pt=G.height,{w:st,h:ct}=zi(nt,s.tip),N=Math.sqrt(st*st+ct*ct)/2*S.stretch+1;H-N<this._dirtyMinX&&(this._dirtyMinX=H-N),F-N<this._dirtyMinY&&(this._dirtyMinY=F-N),H+N>this._dirtyMaxX&&(this._dirtyMaxX=H+N),F+N>this._dirtyMaxY&&(this._dirtyMaxY=F+N),H-N<this._deltaMinX&&(this._deltaMinX=H-N),F-N<this._deltaMinY&&(this._deltaMinY=F-N),H+N>this._deltaMaxX&&(this._deltaMaxX=H+N),F+N>this._deltaMaxY&&(this._deltaMaxY=F+N),this._grain&&!this._grain.perTip&&(H-N<this._grainDeltaMinX&&(this._grainDeltaMinX=H-N),F-N<this._grainDeltaMinY&&(this._grainDeltaMinY=F-N),H+N>this._grainDeltaMaxX&&(this._grainDeltaMaxX=H+N),F+N>this._grainDeltaMaxY&&(this._grainDeltaMaxY=F+N));let K=a,Ct=H,St=F,Pt=null;if(this._grain?.perTip){if(Pt=this._dabBox(H,F,N),!Pt)continue;K=this._scratchFor(Pt),Ct=H-Pt.x,St=F-Pt.y}if(this._colorMode){let it;if(this._dynamicColor)it=this._tintedTipFor(G,r.currentColor);else{if((Mt>this._tintW||pt>this._tintH)&&(this._tintW=Math.max(this._tintW,Mt),this._tintH=Math.max(this._tintH,pt),this._tintCanvas=kt(this._tintW,this._tintH),this._tintedTip=null),G!==this._tintedTip||r.currentColor!==this._tintedColor){const tt=ot(this._tintCanvas);tt.globalCompositeOperation="source-over",tt.clearRect(0,0,this._tintW,this._tintH),ls(tt,G,0,0),Mi(tt,r.currentColor,Mt,pt),this._tintedTip=G,this._tintedColor=r.currentColor}it=this._tintCanvas}K.globalAlpha=Wt,K.globalCompositeOperation="source-over",mn(K,Ct,St,Ht,S,Xt)?(K.drawImage(it,0,0,Mt,pt,-st/2,-ct/2,st,ct),K.restore()):K.drawImage(it,0,0,Mt,pt,Ct-st/2,St-ct/2,st,ct)}else K.globalAlpha=Wt,K.globalCompositeOperation="source-over",mn(K,Ct,St,Ht,S,Xt)?(ls(K,G,-st/2,-ct/2,st,ct),K.restore()):ls(K,G,Ct-st/2,St-ct/2,st,ct);Pt&&this._layGrainedDab(a,K,Pt,l.pressure)}this._prevStamp=l}a.globalAlpha=1}_resetGrainDelta(){this._grainDeltaMinX=1/0,this._grainDeltaMinY=1/0,this._grainDeltaMaxX=-1/0,this._grainDeltaMaxY=-1/0}_dabBox(t,s,i){const r=Math.max(0,Math.floor(t-i)),n=Math.max(0,Math.floor(s-i)),a=Math.min(this._docWidth,Math.ceil(t+i))-r,o=Math.min(this._docHeight,Math.ceil(s+i))-n;return a>0&&o>0?{x:r,y:n,w:a,h:o}:null}_scratchFor(t){(!this._dabScratch||this._dabScratch.width<t.w||this._dabScratch.height<t.h)&&(this._dabScratch=kt(Math.max(t.w,this._dabScratch?.width??0),Math.max(t.h,this._dabScratch?.height??0)));const s=ot(this._dabScratch,{willReadFrequently:!0});return s.clearRect(0,0,t.w,t.h),s}_layGrainedDab(t,s,i,r){const n=this._grain,a=s.getImageData(0,0,i.w,i.h);$r(a.data,i.w,i.h,i.x,i.y,n,n.depthPressure?n.depth*Math.max(0,Math.min(1,r)):n.depth),s.putImageData(a,0,0),t.globalAlpha=1,t.globalCompositeOperation="source-over",t.drawImage(this._dabScratch,0,0,i.w,i.h,i.x,i.y,i.w,i.h)}_grainedBuffer(){const t=this._bufferPool.current;if(!t||!this._grain||this._grain.perTip)return t;if((!this._grainCanvas||this._grainCanvas.width!==this._docWidth||this._grainCanvas.height!==this._docHeight)&&(this._grainCanvas=kt(this._docWidth,this._docHeight)),this._grainDeltaMaxX>=this._grainDeltaMinX){const s=Math.max(0,Math.floor(this._grainDeltaMinX)),i=Math.max(0,Math.floor(this._grainDeltaMinY)),r=Math.min(this._docWidth,Math.ceil(this._grainDeltaMaxX))-s,n=Math.min(this._docHeight,Math.ceil(this._grainDeltaMaxY))-i;if(r>0&&n>0){const a=ot(t).getImageData(s,i,r,n);$r(a.data,r,n,s,i,this._grain),ot(this._grainCanvas).putImageData(a,s,i)}this._resetGrainDelta()}return this._grainCanvas}_rollColor(){const t=this._colorRng;return{hue:t.next(),saturation:t.next(),brightness:t.next(),fgBg:t.next()}}_dabColor(t,s){const i=this._descriptor;if(!i.color.perTip&&this._strokeDabColor!==null)return this._strokeDabColor;const r=wl(this._color,this._secondary||this._color,i.color,t,s);return i.color.perTip||(this._strokeDabColor=r),r}_tintedTipFor(t,s){const i=Math.ceil((t.width+re)/ms)*ms,r=Math.ceil((t.height+re)/ms)*ms;(i>this._slotW||r>this._slotH)&&(this._tintCache.clear(),this._slotW=Math.max(this._slotW,i),this._slotH=Math.max(this._slotH,r));let n=this._tipIds.get(t);n===void 0&&this._tipIds.set(t,n=this._nextTipId++);const a=`${n}|${s}`,o=this._tintCache.get(a);if(o)return this._tintCache.delete(a),this._tintCache.set(a,o),o;const h=Math.max(1,Math.min(Sl,Math.floor(Cl/(this._slotW*this._slotH*4))));let c;if(this._tintCache.size>=h){const u=this._tintCache.keys().next().value;c=this._tintCache.get(u),this._tintCache.delete(u)}c??=kt(this._slotW,this._slotH);const l=ot(c);l.globalCompositeOperation="source-over",l.clearRect(0,0,t.width+re,t.height+re),ls(l,t,0,0),Mi(l,s,t.width,t.height);const d=c;for(let u=0;u<re;u++)l.drawImage(d,t.width-1,0,1,t.height,t.width+u,0,1,t.height);for(let u=0;u<re;u++)l.drawImage(d,0,t.height-1,t.width+re,1,0,t.height+u,t.width+re,1);return this._tintCache.set(a,c),c}_clearTintCache(){this._tintCache.clear(),this._slotW=0,this._slotH=0}commit(t){return this._descriptor?(this._finishSegment(),this._bufferPool.commit(t,this._color,this._descriptor.opacity,this._eraser,this.getDirtyBounds(),this._colorMode,this._descriptor.blendMode,this._alphaLocked,this._grain&&!this._grain.perTip?this._grainedBuffer():null),this._descriptor=null,this._inkState=null,this._unpinTip(),this._clearTintCache(),!0):!1}cancel(){this._descriptor=null,this._inkState=null,this._unpinTip(),this._clearTintCache(),this._smoother.reset(),this._stabilizer.reset()}_finishSegment(){if(!this._descriptor)return;for(const s of this._stabilizer.catchUp())this._stampPoints(this._feed(s));const t=this._smoother.flush(this._dabSpacing);t.length>0&&this._stampPoints(t)}_sizeAt(t){const s=this._descriptor,i=s.dynamics.minSize,r=s.pressureSize?s.size*(i+(1-i)*t.pressure):s.size;return Math.max(1,r*yn(s.dynamics.sizeVelocity,t.velocity)*this._fadeFactor(t))}_fadeFactor(t){const{sizeLength:s,sizeUnit:i,taperStart:r}=this._descriptor.fade;let n=1;if(s>0||r>0){const a=this._descriptor.dynamics.minSize;if(s>0){const o=i==="dabs"?t.dab+this._dabBase:this._distBase+t.pathDist;n*=1-Math.min(1,o/s)*(1-a)}if(r>0){const o=Math.min(1,t.pathDist/r);n*=a+(1-a)*o*o*(3-2*o)}}return n}_leanSpacing(t,s){const i=this._descriptor,r=Hi(t.tiltX,t.tiltY,i.dynamics.tilt);if(r.stretch===1||!Bl.has(i.tip.shape))return 1;const n=i.tip.angle*Math.PI/180,a=Math.max(1,i.tip.aspect),o=i.tip.shape,h=Fa(t,i.tip),c=!["fixed","tilt","twist"].includes(i.tip.orientation),l=d=>{const u=h??(!c||s===null?n:d+n);return gn(o,a,u,r,d)/gn(o,a,u,Ha,d)};return Math.min(1,...(s===null?Hl:[s]).map(l))}_resetMotion(){this._velocity=0,this._velocityLast=null,this._velocityDist=0}_trackVelocity(t,s,i){const r=this._velocityLast;if(this._velocityLast={x:t,y:s,t:r&&!(i>r.t)?r.t:i},!r)return this._normalizedVelocity();this._velocityDist+=Math.hypot(t-r.x,s-r.y);const n=i-r.t;if(!(n>0))return this._normalizedVelocity();const a=this._velocityDist*this._viewScale/n;return this._velocity+=(a-this._velocity)*(1-Math.exp(-n/Pl)),this._velocityDist=0,this._normalizedVelocity()}_normalizedVelocity(){return Math.min(1,this._velocity/Tl)}getDirtyBounds(){if(this._dirtyMaxX<this._dirtyMinX)return null;const t=Math.max(0,Math.floor(this._dirtyMinX)),s=Math.max(0,Math.floor(this._dirtyMinY)),i=Math.min(this._docWidth,Math.ceil(this._dirtyMaxX))-t,r=Math.min(this._docHeight,Math.ceil(this._dirtyMaxY))-s;return i<=0||r<=0?null:{x:t,y:s,w:i,h:r}}_resetPreviewDelta(){this._deltaMinX=1/0,this._deltaMinY=1/0,this._deltaMaxX=-1/0,this._deltaMaxY=-1/0}takePreviewDelta(){if(this._deltaMaxX<this._deltaMinX)return null;const t=Math.max(0,Math.floor(this._deltaMinX)),s=Math.max(0,Math.floor(this._deltaMinY)),i=Math.min(this._docWidth,Math.ceil(this._deltaMaxX))-t,r=Math.min(this._docHeight,Math.ceil(this._deltaMaxY))-s;return this._resetPreviewDelta(),i<=0||r<=0?null:{x:t,y:s,w:i,h:r}}_rawDirtyRect(){if(this._dirtyMaxX<this._dirtyMinX)return null;const t=Math.floor(this._dirtyMinX),s=Math.floor(this._dirtyMinY);return{x:t,y:s,w:Math.ceil(this._dirtyMaxX)-t,h:Math.ceil(this._dirtyMaxY)-s}}getStrokePreview(){return!this._descriptor||!this._bufferPool.current?null:{canvas:this._grainedBuffer(),eraser:this._eraser,opacity:this._descriptor.opacity,mode:this._descriptor.blendMode,alphaLocked:this._alphaLocked,color:this._colorMode?null:this._color,bounds:this.getDirtyBounds()}}}const Xl=96,Wl=100,Qt=new Map,Ke=new Map,je=new Map;function vn(e,t){const s=Qt.get(e);for(s&&s!==t&&URL.revokeObjectURL(s),Qt.delete(e),Qt.set(e,t);Qt.size>Wl;){const i=Qt.entries().next().value;if(!i)break;Qt.delete(i[0]),URL.revokeObjectURL(i[1])}return t}async function Yl(e){const t=await createImageBitmap(e);try{const s=Math.min(1,Xl/Math.max(t.width,t.height)),i=document.createElement("canvas");return i.width=Math.max(1,Math.round(t.width*s)),i.height=Math.max(1,Math.round(t.height*s)),i.getContext("2d").drawImage(t,0,0,i.width,i.height),URL.createObjectURL(await cr(i))}finally{t.close()}}async function Ul(e,t){const s=Qt.get(t.id);if(s)return vn(t.id,s);const i=Ke.get(t.id);if(i)return i;const r=je.get(t.id)??0,n=(async()=>{const a=await e.blobs.get(t.blobRef),o=await Yl(a);if((je.get(t.id)??0)!==r)throw URL.revokeObjectURL(o),new Error("Stamp thumbnail was removed while loading.");return vn(t.id,o)})();Ke.set(t.id,n);try{return await n}finally{Ke.get(t.id)===n&&(Ke.delete(t.id),Qt.has(t.id)||je.delete(t.id))}}function Vl(e){const t=Qt.get(e);t&&URL.revokeObjectURL(t),Qt.delete(e),Ke.has(e)?je.set(e,(je.get(e)??0)+1):je.delete(e)}function ee(e){for(let t=e.getRootNode();t instanceof ShadowRoot;t=t.host.getRootNode()){const s=t.host;if(s.tabIndex>=0)return s.focus({preventScroll:!0}),!0;if(s.localName==="drawing-app")return!1}return!1}function bn(e){for(let t=e.getRootNode();t instanceof ShadowRoot;t=t.host.getRootNode())if(t.host.localName==="drawing-app")return t.host;return null}function Gl(e,t){for(let s=t;s;s=s.parentNode??(s instanceof ShadowRoot?s.host:null))if(s===e)return!0;return!1}function Ds(e,t){const s=bn(e);if(!s)return!0;const i=t.composedPath(),r=i.indexOf(s);for(let n=0;n<(r>=0?r:i.length);n++){const a=i[n];if(a instanceof HTMLDialogElement&&a.open&&bn(a)===s)return!1}return r>=0||t.target===document.body||t.target===document.documentElement}var ql=Object.defineProperty,Kl=Object.getOwnPropertyDescriptor,ut=(e,t,s,i)=>{for(var r=i>1?void 0:i?Kl(t,s):t,n=e.length-1,a;n>=0;n--)(a=e[n])&&(r=(i?a(t,s,r):a(r))||r);return i&&r&&ql(t,s,r),r};const pi={tiltX:0,tiltY:50},wn={rename:W`<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M17 3a2.83 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/></svg>`,delete:W`<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>`},Zl=W`<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/></svg>`,Ql=[{label:"800 × 600",width:800,height:600},{label:"1024 × 768",width:1024,height:768},{label:"1280 × 720 (HD)",width:1280,height:720},{label:"1920 × 1080 (Full HD)",width:1920,height:1080},{label:"2560 × 1440 (QHD)",width:2560,height:1440},{label:"A4 Portrait (794 × 1123)",width:794,height:1123},{label:"A4 Landscape (1123 × 794)",width:1123,height:794},{label:"Square 1024",width:1024,height:1024}],Jl=["#000000","#ffffff","#ff0000","#ff6600","#ffcc00","#33cc33","#0099ff","#6633ff","#cc33cc","#996633","#ff9999","#ffcc99","#ffff99","#99ff99","#99ccff","#cc99ff","#cccccc","#666666"],td=[{id:"basic",label:"Basic"},{id:"artistic",label:"Artistic"},{id:"effects",label:"Effects"}],ed=[{id:"stroke",label:"Stroke"},{id:"tip",label:"Tip"},{id:"stylus",label:"Stylus"},{id:"shape",label:"Shape"},{id:"scatter",label:"Scatter"},{id:"color",label:"Color"},{id:"texture",label:"Texture"},{id:"ink",label:"Ink"}];function sd(e,t){switch(e){case"stylus":return t.pressureSize||t.pressureOpacity||t.dynamics.tilt>0;case"shape":return t.dynamics.sizeVelocity!==0||t.dynamics.flowVelocity!==0||t.fade.taperStart>0||t.fade.sizeLength>0||t.jitter.size>0||t.jitter.angle>0||t.jitter.roundness>0||t.jitter.flow>0;case"scatter":return t.scatter.amount>0||t.scatter.count>1;case"color":return rr(t.color);case"texture":return t.texture.pattern!=="none";case"ink":return t.ink.depletion>0||t.ink.buildup>0||t.ink.wetness>0;default:return!1}}let rt=class extends Bt{constructor(){super(...arguments),this._aspectLock=!1,this._recentStamps=[],this._stampBusy=!1,this._stampMessage="",this._stampMessageError=!1,this._projectDropdownOpen=!1,this._brushPage="stroke",this._brushDropdownOpen=!1,this._tipMessage="",this._tipMessageError=!1,this._openGroups=new Set,this._importProgress=null,this._tipThumbUrls=new Map,this._openPanel=null,this._refocusAfterClose=!1,this._previewCache=new Map,this._tipPreviewCache=new Map,this._customPreviewKey="",this._customPreviewUrl="",this._previewCanvas=null,this._previewSampleCanvas=null,this._previewStrokeCanvas=null,this._previewEngine=new Na,this._renamingProjectId=null,this._newProjectName="Untitled",this._newProjectWidth="800",this._newProjectHeight="600",this._thumbUrls=new Map,this._lastProjectId=null,this._stampLoadVersion=0,this._ctx=new Me(this,{context:Fe,subscribe:!0}),this._storageCtx=new Me(this,{context:ma,subscribe:!0}),this._serviceCtx=new Me(this,{context:ga,subscribe:!0}),this._blurOnEnter=e=>{if(e.key!=="Enter")return;const t=e.target;ee(this),this.shadowRoot?.activeElement===t&&t.blur()},this._onDropdownEscape=e=>{e.key!=="Escape"||!this._projectDropdownOpen&&!this._brushDropdownOpen||!Ds(this,e)||this._renamingProjectId||(e.stopPropagation(),e.preventDefault(),this._closeDropdown(),this._closeBrushDropdown())},this._onDocumentClick=e=>{if(this._projectDropdownOpen){const t=e.composedPath(),s=this.shadowRoot?.querySelector(".project-dropdown-wrap");s&&!t.includes(s)&&this._closeDropdown()}},this._onPanelResize=()=>this._clampOpenPanel(),this._onPanelOutsidePointer=e=>{const t=this._openPanelWrap();t&&!e.composedPath().includes(t)&&this._closePanel()},this._onPanelEscape=e=>{if(e.key!=="Escape"||this._openPanel===null||!Ds(this,e))return;e.stopPropagation(),e.preventDefault();const t=this._openPanelWrap(),s=!!t&&e.composedPath().includes(t);this._closePanel(),s&&t.querySelector(".panel-trigger")?.focus()},this._onPanelFocusOut=e=>{const t=e.relatedTarget;if(!t)return;e.currentTarget.contains(t)||this._closePanel()},this._onBrushDropdownOutsideClick=e=>{const t=e.composedPath(),s=this.shadowRoot?.querySelector(".brush-dropdown-wrap");s&&!t.includes(s)&&this._closeBrushDropdown()},this._defineFromSelection=async()=>{this._showTipMessage(null),this._showTipMessage(await this.ctx.defineBrushFromSelection(),"Brush added to Custom.")},this._defineFromFile=async e=>{const t=e.target,s=t.files?.[0];t.value="",s&&(this._showTipMessage(null),this._showTipMessage(await this.ctx.defineBrushFromFile(s,s.name),"Brush added to Custom."))},this._importBrushes=async e=>{const t=e.target,s=t.files?.[0];if(t.value="",!(!s||this._importProgress!==null)){this._showTipMessage(null),this._importProgress=0;try{const{ok:i,message:r}=await this.ctx.importBrushFile(s,n=>{this._importProgress=n});this._showTipMessage(i?null:r,i?r:"")}finally{this._importProgress=null}}}}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback(),document.addEventListener("keydown",this._onDropdownEscape,!0)}willUpdate(){if(this.toggleAttribute("mobile",this._ctx.value?.isMobile??!1),this._openPanel&&!this._panelAvailable(this._openPanel)){const t=this._openPanelWrap();this._refocusAfterClose=!!t&&t.contains(this.shadowRoot.activeElement),this._closePanel()}const e=this._ctx.value?.currentProject?.id??null;e&&e!==this._lastProjectId&&(this._lastProjectId=e,this._recentStamps=[],this._thumbUrls.clear(),this._stampMessage="",this._stampMessageError=!1,this._loadStamps(e))}disconnectedCallback(){super.disconnectedCallback(),document.removeEventListener("keydown",this._onDropdownEscape,!0),this._closeDropdown(),document.removeEventListener("click",this._onBrushDropdownOutsideClick),this._closePanel(),this._stampLoadVersion++,this._thumbUrls.clear(),this._lastProjectId=null;const e=this.shadowRoot?.querySelector(".new-project-dialog");e?.open&&e.close()}async _loadStamps(e){const t=this._storageCtx.value;if(!t)return;const s=++this._stampLoadVersion;try{const i=await t.stamps.list(e);if(this._lastProjectId!==e||s!==this._stampLoadVersion)return;this._recentStamps=i;const r=new Set(i.map(n=>n.id));for(const n of this._thumbUrls.keys())r.has(n)||this._thumbUrls.delete(n);for(const n of i){if(this._lastProjectId!==e||s!==this._stampLoadVersion)return;this._thumbUrls.has(n.id)||(this._thumbUrls.set(n.id,await Ul(t,n)),this._lastProjectId===e&&s===this._stampLoadVersion&&(this._recentStamps=[...i]))}this._lastProjectId===e&&s===this._stampLoadVersion&&(this._recentStamps=i)}catch(i){this._lastProjectId===e&&s===this._stampLoadVersion&&(this._stampMessage=i instanceof Error?i.message:"Could not load recent stamps.",this._stampMessageError=!0)}}_onStrokeColor(e){this.ctx.setStrokeColor(e.target.value)}_onFillColor(e){this.ctx.setFillColor(e.target.value)}_onBrushSize(e){this.ctx.setBrushSize(Number(e.target.value))}_onStampSize(e){this.ctx.setStampSize(Number(e.target.value))}_onUseFill(e){this.ctx.setUseFill(e.target.checked)}_uploadStamp(){if(!this._ctx.value?.currentProject?.id)return;const t=document.createElement("input");t.type="file",t.accept="image/*",t.multiple=!0,t.onchange=async()=>{const s=t.files;if(!s||s.length===0)return;const i=this._ctx.value?.currentProject?.id;if(!i)return;const r=this._serviceCtx.value;if(!r)return;const n=10*1024*1024,a=4096;let o=null,h=0,c=0;this._stampBusy=!0,this._stampMessage=`Importing ${s.length===1?s[0].name:`${s.length} images`}…`,this._stampMessageError=!1;try{for(const l of s){if(this._ctx.value?.currentProject?.id!==i)break;if(l.size>n){c++;continue}try{const d=await createImageBitmap(l);if(d.width>a||d.height>a){c++,d.close();continue}d.close()}catch{c++;continue}o=await r.addStamp(i,l),h++}await this._loadStamps(i),o&&this._lastProjectId===i&&(this._stampBusy=!1,await this._selectStamp(o,!1)),this._stampMessage=h>0?`${h} ${h===1?"stamp":"stamps"} added${c?`; ${c} skipped (10 MB / 4096 px limit).`:"."}`:"No stamps were added. Use a valid image up to 10 MB and 4096 px per side.",this._stampMessageError=h===0}catch(l){this._stampMessage=l instanceof Error?l.message:"Could not add the stamp.",this._stampMessageError=!0}finally{this._stampBusy=!1}},t.click()}async _selectStamp(e,t=!0){const s=this._storageCtx.value,i=this._ctx.value?.currentProject?.id??null;if(!(!s||this._stampBusy||!i||e.projectId!==i||this._lastProjectId!==i)){this._stampBusy=!0,t&&(this._stampMessage="Loading stamp…",this._stampMessageError=!1);try{const r=await s.blobs.get(e.blobRef),n=URL.createObjectURL(r);let a;try{a=await new Promise((o,h)=>{const c=new Image;c.onload=()=>o(c),c.onerror=()=>h(new Error("The stamp image could not be decoded.")),c.src=n})}finally{URL.revokeObjectURL(n)}if(this._lastProjectId!==i||this._ctx.value?.currentProject?.id!==i)return;this.ctx.setStampImage(a,e.id),t&&(this._stampMessage="Stamp selected. Click the canvas to place it.")}catch(r){this._stampMessage=r instanceof Error?r.message:"Could not load the stamp.",this._stampMessageError=!0}finally{this._stampBusy=!1}}}async _deleteStamp(e,t){t.stopPropagation();const s=this._ctx.value?.currentProject?.id;if(!s||e.projectId!==s||this._lastProjectId!==s)return;const i=this._storageCtx.value;if(i)try{if(await i.stamps.delete(e.id),Vl(e.id),this._ctx.value?.currentProject?.id!==s||this._lastProjectId!==s)return;this.ctx.state.activeStampId===e.id&&this.ctx.setStampImage(null,null),await this._loadStamps(s),this._stampMessage="Stamp removed.",this._stampMessageError=!1}catch(r){this._stampMessage=r instanceof Error?r.message:"Could not remove the stamp.",this._stampMessageError=!0}}_closeDropdown(){this._projectDropdownOpen&&(this._projectDropdownOpen=!1,document.removeEventListener("click",this._onDocumentClick))}_toggleProjectDropdown(){this._projectDropdownOpen?this._closeDropdown():(this._projectDropdownOpen=!0,document.addEventListener("click",this._onDocumentClick))}_onSelectProject(e){this._closeDropdown(),this.ctx.switchProject(e)}_onNewProject(){this._closeDropdown(),this._newProjectName="Untitled",this._newProjectWidth="800",this._newProjectHeight="600";const e=this.shadowRoot?.querySelector(".new-project-dialog");e?.open&&e.close(),e?.showModal(),this.updateComplete.then(()=>{const t=this.shadowRoot?.querySelector(".new-project-name-input");t&&(t.focus(),t.select())})}_cancelNewProject(){this.shadowRoot?.querySelector(".new-project-dialog")?.close()}_confirmNewProject(){const e=this._newProjectName.trim()||"Untitled",t=parseInt(this._newProjectWidth),s=parseInt(this._newProjectHeight);if(!t||!s||t<=0||s<=0||t>8192||s>8192)return;this.shadowRoot?.querySelector(".new-project-dialog")?.close(),this.ctx.createProject(e,t,s)}_selectNewProjectPreset(e){this._newProjectWidth=String(e.width),this._newProjectHeight=String(e.height)}_onNewProjectKeydown(e){e.stopPropagation(),e.key==="Enter"&&this._confirmNewProject()}async _onDeleteProject(e,t){e.stopPropagation();const s=this.ctx.projectList.find(i=>i.id===t)?.name??"this project";await this.ctx.confirm({message:`Delete “${s}”? This cannot be undone.`,confirmLabel:"Delete",danger:!0})&&(this._closeDropdown(),this.ctx.deleteProject(t))}_startRename(e,t){e.stopPropagation(),this._renamingProjectId=t,this.updateComplete.then(()=>{const s=this.shadowRoot?.querySelector(".project-rename-input");s&&(s.focus(),s.select())})}_onRenameKeydown(e,t){e.stopPropagation(),e.key==="Enter"?(this._commitRename(e,t),ee(this)):e.key==="Escape"&&(e.preventDefault(),this._renamingProjectId=null,ee(this))}_commitRename(e,t){if(this._renamingProjectId!==t)return;const i=e.target.value.trim();i&&this.ctx.renameProject(t,i),this._renamingProjectId=null}_showsShapeOptions(){const e=this.ctx.state.activeTool;return ce(e)&&e!=="line"}_generatePreview(e,t=!1){const s=`${t?"eraser":"paint"}:${e.id}:${xs(e.descriptor.tip.imageId)}`,i=this._previewCache.get(s);if(i)return i;const r=this._generateDescriptorPreview(e.descriptor,t);return this._previewCache.set(s,r),r}_tipPreview(e){const t=e.tip,s=[t.shape,t.imageId??"",xs(t.imageId),e.hardness.toFixed(2),t.aspect,t.orientation==="fixed"?t.angle:0,t.bristles??"",t.spread??""].join(":"),i=this._tipPreviewCache.get(s);if(i)return i;const r=84,n=54,a=4,o=document.createElement("canvas");o.width=r,o.height=n;const h=o.getContext("2d");if(h.fillStyle="#2a2a2a",h.fillRect(0,0,r,n),!la(t)){const l=t.orientation==="fixed"?t.angle*Math.PI/180:0,{w:d,h:u}=zi(n,t),_=Math.abs(Math.cos(l)),f=Math.abs(Math.sin(l)),m=Math.min((r-2*a)/(d*_+u*f),(n-2*a)/(d*f+u*_)),p=ji[t.shape](Math.max(2,Math.round(Math.max(d,u)*m*2)),e.hardness,t),g=document.createElement("canvas");g.width=p.width,g.height=p.height;const y=g.getContext("2d");y.drawImage(p,0,0),y.globalCompositeOperation="source-in",y.fillStyle="#cccccc",y.fillRect(0,0,g.width,g.height),h.translate(r/2,n/2),h.rotate(l),h.drawImage(g,-d*m/2,-u*m/2,d*m,u*m)}const c=o.toDataURL();return this._tipPreviewCache.set(s,c),c}_generateCustomPreview(e,t=!1){const s=`${t?"eraser":"paint"}:${xs(e.tip.imageId)}:${JSON.stringify(e)}`;return s===this._customPreviewKey?this._customPreviewUrl:(this._customPreviewKey=s,this._customPreviewUrl=this._generateDescriptorPreview(e,t),this._customPreviewUrl)}_generateDescriptorPreview(e,t=!1){const r=this._previewCanvas??=document.createElement("canvas");r.width!==160&&(r.width=160),r.height!==48&&(r.height=48);const n=r.getContext("2d");n.globalAlpha=1,n.globalCompositeOperation="source-over",n.clearRect(0,0,160,48),n.fillStyle="#2a2a2a",n.fillRect(0,0,160,48);const a=12,o=Math.max(2,Math.min(18,2+Math.log2(Math.max(1,e.size))*2.5)),h=e.tip.shape==="image",c=8,{w:l,h:d}=zi(1,e.tip),u=e.tip.angle*Math.PI/180,f=e.tip.orientation!=="fixed"||e.jitter.angle>0?Math.hypot(l,d)/2:(Math.abs(l*Math.sin(u))+Math.abs(d*Math.cos(u)))/2,m=h?Math.max(2,Math.min(160/3,(48/2-2-c)/(f+e.scatter.amount))):e.scatter.amount>0?Math.max(2,Math.min(o,a/e.scatter.amount)):o,p={...Oe(e),size:m,blendMode:"normal"},g=8,y=(h?m*f:m/2)*Hi(pi.tiltX,pi.tiltY,p.dynamics.tilt).stretch+2,w=Math.max(0,48/2-y),x=Math.max(2,(w-p.scatter.amount*m)*.8),b=48/2,S=160-g*2,E=this._previewSampleCanvas??=document.createElement("canvas");E.width!==160&&(E.width=160),E.height!==48&&(E.height=48);const D=E.getContext("2d");D.globalAlpha=1,D.globalCompositeOperation="source-over",D.clearRect(0,0,160,48),D.fillStyle="#ef6c57",D.fillRect(0,0,160/2,48),D.fillStyle="#5b8cf7",D.fillRect(160/2,0,160/2,48);const L=this._previewStrokeCanvas??=document.createElement("canvas");L.width!==160&&(L.width=160),L.height!==48&&(L.height=48);const I=L.getContext("2d");I.globalAlpha=1,I.globalCompositeOperation="source-over",I.clearRect(0,0,160,48),t&&(I.fillStyle="#cccccc",I.fillRect(0,0,160,48));const k=!t&&rr(p.color);this._previewEngine.begin(p,k?"#e8a23a":"#cccccc",t,160,48,{seed:1,secondaryColor:k?"#3a8ce8":void 0});let M=0;for(let P=g;P<=160-g;P+=1){const R=(P-g)/S,O=b+x*Math.sin(R*Math.PI*2),V=.4+.6*Math.sin(R*Math.PI);M+=R>.35&&R<.65?4:1;const H=!t&&p.ink.wetness>0?D:void 0;this._previewEngine.stroke(P,O,V,H,M,pi)}return this._previewEngine.commit(I),n.drawImage(L,0,0),r.toDataURL()}updated(){this._refocusAfterClose&&(this._refocusAfterClose=!1,this.shadowRoot?.querySelector(".panel-trigger, .project-name-btn")?.focus()),this._clampOpenPanel()}_clampOpenPanel(){const e=this._openPanelWrap()?.querySelector(".settings-panel");if(!e)return;e.style.left="";const t=8,s=e.getBoundingClientRect(),i=s.right-(window.innerWidth-t);i>0&&(e.style.left=`${-Math.min(i,Math.max(0,s.left-t))}px`)}_panelAvailable(e){const t=this._ctx.value;if(!t||t.isMobile)return!1;const s=t.state.activeTool;return e==="brush"?s==="pencil"||s==="eraser":s!=="select"&&s!=="eraser"&&s!=="stamp"}_togglePanel(e){if(this._openPanel===e){this._closePanel();return}this._openPanel=e,document.addEventListener("pointerdown",this._onPanelOutsidePointer,!0),document.addEventListener("keydown",this._onPanelEscape,!0),window.addEventListener("resize",this._onPanelResize)}_closePanel(){this._openPanel!==null&&(this._openPanel=null,document.removeEventListener("pointerdown",this._onPanelOutsidePointer,!0),document.removeEventListener("keydown",this._onPanelEscape,!0),window.removeEventListener("resize",this._onPanelResize))}_openPanelWrap(){return this._openPanel===null?null:this.shadowRoot?.querySelector(`.panel-wrap[data-panel="${this._openPanel}"]`)??null}_renderColorControls(e){const t=v`
      <input
        type="color"
        .value=${e}
        @input=${this._onStrokeColor}
        title="Stroke color"
        aria-label="Stroke color"
      />
    `,s=v`
      <div class="color-grid">
        ${Jl.map(i=>v`
            <button
              class="color-swatch ${e===i?"active":""}"
              style="background:${i}"
              title=${i}
              aria-label=${`Use ${i}`}
              @click=${()=>this.ctx.setStrokeColor(i)}
            ></button>
          `)}
      </div>
    `;return this.ctx.isMobile?v`${t}${s}`:v`
      ${s}
      <label class="custom-color">${t} Custom color…</label>
    `}_toggleBrushDropdown(){this._brushDropdownOpen=!this._brushDropdownOpen,this._brushDropdownOpen?(this.ctx.loadUserTips?.(["",...this._openGroups]),requestAnimationFrame(()=>{document.addEventListener("click",this._onBrushDropdownOutsideClick)})):document.removeEventListener("click",this._onBrushDropdownOutsideClick)}_closeBrushDropdown(){this._brushDropdownOpen=!1,document.removeEventListener("click",this._onBrushDropdownOutsideClick)}_selectPreset(e){this.ctx.selectPreset(e),this._closeBrushDropdown()}_renderTransformSettings(){const e=this._ctx.value?.getTransformValues();if(!e)return v`<div class="section"><label>No transform active</label></div>`;const{x:t,y:s,width:i,height:r,rotation:n,skewX:a,skewY:o,flipH:h,flipV:c}=e,l=(_,f)=>this._ctx.value?.setTransformValue(_,f),d=this._blurOnEnter,u=(_,f)=>m=>{const p=m.target,g=p.value,y=parseFloat(g);if(!isNaN(y))if(_==="width"&&this._aspectLock&&i!==0){const x=r/i;l("width",y),l("height",Math.round(y*x*10)/10)}else if(_==="height"&&this._aspectLock&&r!==0){const x=i/r;l("height",y),l("width",Math.round(y*x*10)/10)}else l(_,y);const w=this._ctx.value?.getTransformValues()?.[_];typeof w=="number"&&(p.value=String(Math.round(w*10)/10))};return v`
      <div class="transform-section">
        <label>Position</label>
        <div class="transform-row">
          <span class="transform-suffix">X</span>
          <input class="transform-input" @keydown=${d} type="number" step="0.1"
            .value=${String(Math.round(t*10)/10)}
            aria-label="X position" @change=${u("x")} />
          <span class="transform-suffix">Y</span>
          <input class="transform-input" @keydown=${d} type="number" step="0.1"
            .value=${String(Math.round(s*10)/10)}
            aria-label="Y position" @change=${u("y")} />
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Size</label>
        <div class="transform-row">
          <span class="transform-suffix">W</span>
          <input class="transform-input" @keydown=${d} type="number" step="0.1" min="1"
            .value=${String(Math.round(i*10)/10)}
            aria-label="Width" @change=${u("width")} />
          <button
            class="aspect-lock-btn ${this._aspectLock?"active":""}"
            title="Lock aspect ratio"
            @click=${()=>{this._aspectLock=!this._aspectLock}}
          >
            <svg viewBox="0 0 10 14" width="10" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              ${this._aspectLock?v`<rect x="1" y="5" width="8" height="8" rx="1"/><path d="M3 5V3.5a2 2 0 1 1 4 0V5"/>`:v`<rect x="1" y="5" width="8" height="8" rx="1"/><path d="M3 5V3.5a2 2 0 1 1 4 0V4" stroke-dasharray="2 1"/>`}
            </svg>
          </button>
          <span class="transform-suffix">H</span>
          <input class="transform-input" @keydown=${d} type="number" step="0.1" min="1"
            .value=${String(Math.round(r*10)/10)}
            aria-label="Height" @change=${u("height")} />
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Rotation</label>
        <div class="transform-row">
          <input class="transform-input" @keydown=${d} type="number" step="0.1"
            .value=${String(Math.round(n*10)/10)}
            aria-label="Rotation in degrees" @change=${u("rotation","°")} />
          <span class="transform-suffix">°</span>
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Skew</label>
        <div class="transform-row">
          <span class="transform-suffix">X</span>
          <input class="transform-input" @keydown=${d} type="number" step="0.1"
            .value=${String(Math.round(a*10)/10)}
            aria-label="Horizontal skew in degrees" @change=${u("skewX","°")} />
          <span class="transform-suffix">°</span>
          <span class="transform-suffix" style="margin-left:0.25rem;">Y</span>
          <input class="transform-input" @keydown=${d} type="number" step="0.1"
            .value=${String(Math.round(o*10)/10)}
            aria-label="Vertical skew in degrees" @change=${u("skewY","°")} />
          <span class="transform-suffix">°</span>
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Flip</label>
        <div class="transform-row">
          <button
            class="flip-btn ${h?"active":""}"
            title="Flip Horizontal"
            @click=${()=>l("flipH",!0)}
          >
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M8 2v12M2 5l4 3-4 3M14 5l-4 3 4 3"/>
            </svg>
          </button>
          <button
            class="flip-btn ${c?"active":""}"
            title="Flip Vertical"
            @click=${()=>l("flipV",!0)}
          >
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M2 8h12M5 2l3 4 3-4M5 14l3-4 3 4"/>
            </svg>
          </button>
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Brush</label>
        <div class="transform-row">
          <button
            class="pill-btn"
            title="Make a brush tip from the selection: dark parts paint, light and transparent parts don't."
            @click=${this._defineFromSelection}
          >Define brush</button>
        </div>
        ${this._renderTipMessage()}
      </div>
    `}_renderTipMessage(){return this._tipMessage?v`<div class="tip-message ${this._tipMessageError?"error":""}" role="status">${this._tipMessage}</div>`:A}_showTipMessage(e,t=""){this._tipMessage=e??t,this._tipMessageError=e!==null}_toggleGroup(e){const t=new Set(this._openGroups);t.has(e)?t.delete(e):(t.add(e),this.ctx.loadUserTips?.([e])),this._openGroups=t}_renderCustomPresets(e){const t=e.filter(i=>!i.group),s=[...new Set(e.flatMap(i=>i.group?[i.group]:[]))];return v`
      ${t.length>0?v`
        <div class="brush-dropdown-heading">Custom</div>
        ${t.map(i=>this._renderPresetRow(i,!0))}
      `:A}
      ${s.map(i=>{const r=this._openGroups.has(i),n=e.filter(a=>a.group===i);return v`
          <div class="brush-dropdown-heading group-heading">
            <button class="group-toggle" aria-expanded=${r?"true":"false"} title=${i}
              @click=${()=>this._toggleGroup(i)}>
              <span aria-hidden="true">${r?"▾":"▸"}</span> ${i} (${n.length})
            </button>
            <button class="brush-dropdown-delete" title="Delete these brushes" aria-label=${`Delete brushes imported from ${i}`}
              @click=${()=>{this.ctx.deleteUserGroup(i)}}>&times;</button>
          </div>
          ${r?n.map(a=>this._renderPresetRow(a,!0)):A}
        `})}
    `}_tipThumbUrl(e){const t=this._tipThumbUrls.get(e.id);if(t)return t;const{width:s,height:i,alpha:r}=e.thumb,n=document.createElement("canvas");n.width=s,n.height=i;const a=n.getContext("2d"),o=a.createImageData(s,i);for(let c=0,l=0;c<r.length;c++,l+=4)o.data[l]=o.data[l+1]=o.data[l+2]=255-r[c],o.data[l+3]=255;a.putImageData(o,0,0);const h=n.toDataURL();return this._tipThumbUrls.set(e.id,h),h}_renderTipLibrary(){const e=(this.ctx.userTips??[]).filter(s=>!s.group),t=this.ctx.state.brush.tip.shape==="image"?this.ctx.state.brush.tip.imageId:void 0;return v`
      <div class="section">
        <label>Custom</label>
        <div class="pill-row">
          <button class="pill-btn" title="Make a brush tip from an image: dark parts paint, light and transparent parts don't."
            @click=${s=>s.currentTarget.nextElementSibling?.click()}
          >From image…</button>
          <input type="file" accept="image/*" hidden aria-label="Image to make a brush tip from" @change=${this._defineFromFile} />
          <button class="pill-btn" ?disabled=${this._importProgress!==null}
            title="Import the brushes of a Photoshop brush file (.abr): tips, spacing, dynamics, scattering, texture and colour settings."
            @click=${s=>s.currentTarget.nextElementSibling?.click()}
          >Import brushes (.abr)…</button>
          <input type="file" accept=".abr" hidden aria-label="Photoshop brush file to import" @change=${this._importBrushes} />
        </div>
      </div>
      ${this._importProgress!==null?v`
        <progress class="import-progress" max="1" .value=${this._importProgress} aria-label="Importing brushes"></progress>
      `:A}
      ${this._renderTipMessage()}
      ${e.length>0?v`
        <div class="tip-grid" role="list">
          ${e.map(s=>v`
            <div class="tip-cell" role="listitem">
              <button class="tip-pick ${t===s.id?"active":""}" title=${s.name} aria-label=${`Use tip ${s.name}`}
                aria-pressed=${t===s.id?"true":"false"}
                @click=${()=>this.ctx.selectUserTip(s.id)}>
                <img src=${this._tipThumbUrl(s)} alt="" />
              </button>
              <button class="tip-delete" title="Delete tip" aria-label=${`Delete tip ${s.name}`}
                @click=${()=>{this.ctx.deleteUserTip(s.id)}}>&times;</button>
            </div>
          `)}
        </div>
      `:A}
    `}_renderPresetRow(e,t){const{state:s}=this.ctx,i=s.activeTool==="eraser";return v`
      <div class="brush-dropdown-row">
        <button
          class="brush-dropdown-item ${s.activePreset===e.id&&!s.isPresetModified?"active":""}"
          @click=${()=>this._selectPreset(e.id)}
        >
          <img class="tip-thumb" src=${this._tipPreview(e.descriptor)} alt="" />
          <img src=${this._generatePreview(e,i)} alt="" />
          <span>${e.name}</span>
        </button>
        ${t?v`
          <button class="brush-dropdown-delete" title="Delete brush" aria-label=${`Delete brush ${e.name}`}
            @click=${()=>{this.ctx.deleteUserPreset(e.id)}}>&times;</button>
        `:A}
      </div>
    `}_onBrushTabKey(e,t){const s=t.indexOf(this._brushPage),i={ArrowDown:1,ArrowRight:1,ArrowUp:-1,ArrowLeft:-1}[e.key];let r;if(i!==void 0)r=(Math.max(0,s)+i+t.length)%t.length;else if(e.key==="Home")r=0;else if(e.key==="End")r=t.length-1;else return;e.preventDefault(),this._brushPage=t[r],this.updateComplete.then(()=>this.shadowRoot?.querySelector(`#brush-tab-${t[r]}`)?.focus())}_renderBrushDetails(){const{brush:e,activeTool:t,stabilizer:s}=this.ctx.state,i=Fo(e.tip.shape),r=Q(e.tip.shape),n=(p,g,y)=>{const w=Math.round(e.dynamics[g]*100);return v`
        <div class="section">
          <label title=${y}>${p}</label>
          <input type="range" min="-100" max="100" aria-label=${p} .value=${String(w)}
            @input=${x=>this.ctx.setBrushDynamics({[g]:Number(x.target.value)/100})} />
          <span class="size-value">${w>0?"+":""}${w}%</span>
        </div>
      `},a=(p,g,y,w)=>{const x=Math.round(e.fade[g]);return v`
        <div class="section">
          <label title=${w}>${p}</label>
          <input type="range" min="0" max=${y} step="5" aria-label=${p} .value=${String(Math.min(y,x))}
            @input=${b=>this.ctx.setBrushFade({[g]:Number(b.target.value)})} />
          <span class="size-value">${x>0?`${x}${g==="sizeLength"&&e.fade.sizeUnit==="dabs"?" dabs":"px"}`:"Off"}</span>
        </div>
      `},o=(p,g,y)=>{const w=Math.round(e.jitter[g]*100);return v`
        <div class="section">
          <label title=${y}>${p}</label>
          <input type="range" min="0" max="100" aria-label=${`${p} jitter`} .value=${String(w)}
            @input=${x=>this.ctx.setBrushJitter({[g]:Number(x.target.value)/100})} />
          <span class="size-value">${w}%</span>
        </div>
      `},h=(p,g,y,w,x,b,S)=>v`
      <div class="section">
        <label title=${y}>${p}</label>
        <input type="range" min=${w} max=${x} aria-label=${`${p} scatter`} .value=${String(b)}
          @input=${E=>this.ctx.setBrushScatter({[g]:S(Number(E.target.value))})} />
        <span class="size-value">${g==="count"?b:`${b}%`}</span>
      </div>
    `,c=(p,g,y,w=0)=>v`
      <div class="section">
        <label title=${y}>${p}</label>
        <input type="range" min=${w} max="100" aria-label=${`${p} color`} .value=${String(Math.round(e.color[g]*100))}
          @input=${x=>this.ctx.setBrushColorDynamics({[g]:Number(x.target.value)/100})} />
        <span class="size-value">${Math.round(e.color[g]*100)}%</span>
      </div>
    `,l=(p,g,y)=>v`
      <div class="section">
        <label class="checkbox-label" title=${y}>
          <input type="checkbox" aria-label=${`${p} color`} .checked=${e.color[g]}
            @change=${w=>this.ctx.setBrushColorDynamics({[g]:w.target.checked})} />
          ${p}
        </label>
      </div>
    `,d=(p,g,y,w,x)=>v`
      <div class="section">
        <label title=${y}>${p}</label>
        <input type="range" min=${w} max=${x} aria-label=${`${p} texture`} .value=${String(Math.round(e.texture[g]*100))}
          @input=${b=>this.ctx.setBrushTexture({[g]:Number(b.target.value)/100})} />
        <span class="size-value">${Math.round(e.texture[g]*100)}%</span>
      </div>
    `,u=(p,g,y)=>v`
      <div class="section">
        <label class="checkbox-label" title=${y}>
          <input type="checkbox" aria-label=${`${p} texture`} .checked=${e.texture[g]}
            @change=${w=>this.ctx.setBrushTexture({[g]:w.target.checked})} />
          ${p}
        </label>
      </div>
    `,_={stroke:()=>v`
      ${t!=="eraser"?v`
      <div class="section">
        <label title="How the stroke combines with the layer. Behind paints only under what is already there; Clear erases. On a layer with its transparency locked, Behind and Clear do nothing.">Mode</label>
        <select class="font-select" aria-label="Brush mode" .value=${e.blendMode}
          @change=${p=>this.ctx.setBrush({blendMode:p.target.value})}>
          ${Object.keys(ki).map(p=>v`<option value=${p} ?selected=${e.blendMode===p}>${ki[p]}</option>`)}
        </select>
      </div>
      `:A}
      <div class="section">
        <label>Flow</label>
        <input type="range" aria-label="Brush flow" min="1" max="100" .value=${String(Math.round(e.flow*100))}
          @input=${p=>this.ctx.setBrush({flow:Number(p.target.value)/100})} />
        <span class="size-value">${Math.round(e.flow*100)}%</span>
      </div>
      <div class="section">
        <label class="checkbox-label" title="Keep laying dabs while the pen is held still or moves slowly, so paint builds up toward the opacity. Needs a flow under 100%.">
          <input type="checkbox" aria-label="Airbrush" .checked=${e.airbrush}
            @change=${p=>this.ctx.setBrush({airbrush:p.target.checked})} />
          Airbrush
        </label>
      </div>
      ${e.airbrush?v`
      <div class="section">
        <label title="How many dabs a held airbrush lays per second.">Rate</label>
        <input type="range" aria-label="Airbrush rate" min=${Jn} max=${ta} .value=${String(Math.round(e.airbrushRate))}
          @input=${p=>this.ctx.setBrush({airbrushRate:ir(Number(p.target.value))})} />
        <span class="size-value">${Math.round(e.airbrushRate)}/s</span>
      </div>
      `:A}
      <div class="section">
        <label title="Steadies the line: it trails the pointer, more so the higher this is.">Stabilizer</label>
        <input type="range" min="0" max="100" aria-label="Stabilizer" .value=${String(Math.round(s))}
          @input=${p=>this.ctx.setStabilizer(Number(p.target.value))} />
        <span class="size-value">${Math.round(s)}%</span>
      </div>
      `,tip:()=>v`
      <div class="section">
        <label>Tip</label>
        <div class="pill-row">
          ${["round","flat","chisel","calligraphy","fan","splatter"].map(p=>v`
            <button
              class="pill-btn ${e.tip.shape===p?"active":""}"
              @click=${()=>{e.tip.shape!==p&&this.ctx.setBrushTip(Q(p))}}
            >${p.charAt(0).toUpperCase()+p.slice(1)}</button>
          `)}
        </div>
      </div>
      ${this._renderTipLibrary()}
      ${i.hardness?v`<div class="section">
        <label>Hardness</label>
        <input type="range" aria-label="Brush hardness" min="0" max="100" .value=${String(Math.round(e.hardness*100))}
          @input=${p=>this.ctx.setBrush({hardness:Number(p.target.value)/100})} />
        <span class="size-value">${Math.round(e.hardness*100)}%</span>
      </div>`:A}
      <div class="section">
        <label>Spacing</label>
        <input type="range" min="5" max="100" aria-label="Spacing" .value=${String(Math.round(e.spacing*100))}
          @input=${p=>this.ctx.setBrush({spacing:Number(p.target.value)/100})} />
        <span class="size-value">${Math.round(e.spacing*100)}%</span>
      </div>
      ${i.aspect?v`
      <div class="section">
        <label>Aspect</label>
        <input type="range" min="1" max="6" step="0.5" .value=${String(e.tip.aspect)}
          @input=${p=>this.ctx.setBrushTip({aspect:Number(p.target.value)})} />
        <span class="size-value">${e.tip.aspect}</span>
      </div>
      `:A}
      ${i.rotation?v`
      <div class="section">
        <label>${e.tip.orientation==="fixed"?"Angle":"Offset"}</label>
        <input type="range" min="0" max="360" .value=${String(e.tip.angle)}
          @input=${p=>this.ctx.setBrushTip({angle:Number(p.target.value)})} />
        <span class="size-value">${e.tip.angle}&deg;</span>
      </div>
      <div class="section">
        <label>Orient</label>
        <select class="font-select" aria-label="Tip orientation" .value=${e.tip.orientation}
          @change=${p=>this.ctx.setBrushTip({orientation:p.target.value})}>
          <option value="fixed">Fixed</option>
          <option value="direction">Direction</option>
          <option value="tilt">Pen tilt</option>
          <option value="twist">Pen rotation</option>
        </select>
      </div>
      `:A}
      ${i.bristles?v`
        <div class="section">
          <label>Bristles</label>
          <input type="range" min="1" max="20" .value=${String(e.tip.bristles??r.bristles)}
            @input=${p=>this.ctx.setBrushTip({bristles:Number(p.target.value)})} />
          <span class="size-value">${e.tip.bristles??r.bristles}</span>
        </div>
        <div class="section">
          <label>Spread</label>
          <input type="range" min="0" max="200" .value=${String(Math.round((e.tip.spread??r.spread)*(e.tip.shape==="fan"?1:100)))}
            @input=${p=>{const g=Number(p.target.value);this.ctx.setBrushTip({spread:e.tip.shape==="fan"?g:g/100})}} />
          <span class="size-value">${e.tip.shape==="fan"?e.tip.spread??r.spread:Math.round((e.tip.spread??r.spread)*100)+"%"}</span>
        </div>
      `:A}
      `,stylus:()=>v`
      <div class="section">
        <label class="checkbox-label" title="Affects pressure-sensitive pen or stylus input; mouse input uses full pressure.">
          <input type="checkbox" .checked=${e.pressureSize}
            @change=${p=>this.ctx.setBrush({pressureSize:p.target.checked})} />
          Stylus Size
        </label>
      </div>
      <div class="section">
        <label class="checkbox-label" title="Affects pressure-sensitive pen or stylus input; mouse input uses full pressure.">
          <input type="checkbox" .checked=${e.pressureOpacity}
            @change=${p=>this.ctx.setBrush({pressureOpacity:p.target.checked})} />
          Stylus Opacity
        </label>
      </div>
      <div class="section">
        <label title="Stretches dabs along the way a tilting pen or stylus leans; upright, mouse and touch input are unaffected.">Stylus Tilt</label>
        <input type="range" min="0" max="100" aria-label="Stylus tilt" .value=${String(Math.round(e.dynamics.tilt*100))}
          @input=${p=>this.ctx.setBrushDynamics({tilt:Number(p.target.value)/100})} />
        <span class="size-value">${Math.round(e.dynamics.tilt*100)}%</span>
      </div>
      ${e.pressureSize||e.pressureOpacity?v`
      <div class="section">
        <label title="Maps pressure-sensitive pen or stylus input; mouse input uses full pressure.">Stylus Curve</label>
        <select class="font-select" aria-label="Stylus curve" .value=${e.pressureCurve}
          @change=${p=>this.ctx.setBrush({pressureCurve:p.target.value})}>
          <option value="linear">Linear</option>
          <option value="light">Light</option>
          <option value="heavy">Heavy</option>
        </select>
      </div>
      `:A}
      `,shape:()=>v`
      ${e.pressureSize||e.fade.sizeLength>0||e.fade.taperStart>0?v`
      <div class="section">
        <label title="How thick the lightest touch draws, as a share of the size; also the smallest size the taper and size fade reach.">Minimum Size</label>
        <input type="range" min="0" max="100" aria-label="Minimum size" .value=${String(Math.round(e.dynamics.minSize*100))}
          @input=${p=>this.ctx.setBrushDynamics({minSize:Number(p.target.value)/100})} />
        <span class="size-value">${Math.round(e.dynamics.minSize*100)}%</span>
      </div>
      `:A}
      ${n("Size by speed","sizeVelocity","Positive: dabs grow the faster you draw. Negative: they shrink.")}
      ${n("Flow by speed","flowVelocity","Positive: ink flows more the faster you draw. Negative: less.")}
      ${a("Taper start","taperStart",500,"The dab grows from its smallest size to full over this distance at the start of each stroke, to a point. The smallest size is set by Minimum Size.")}
      ${a("Size fade","sizeLength",2e3,"The dab shrinks along the stroke, reaching its smallest size at this distance. Flow fades with Depletion under Ink.")}
      ${o("Size","size","How much smaller than the brush size each dab can randomly be.")}
      ${o("Angle","angle","How far each dab can randomly turn, as a share of a full turn.")}
      ${o("Roundness","roundness","How much each dab can randomly be squashed across its axis.")}
      ${o("Flow","flow","How much fainter than the brush flow each dab can randomly be. The stroke opacity still caps the result.")}
      `,scatter:()=>v`
      ${h("Scatter","amount","How far dabs can land from the stroke, to either side, as a share of the brush size.",0,er*100,Math.round(e.scatter.amount*100),p=>p/100)}
      <div class="section">
        <label class="checkbox-label" title="Scatter along the stroke as well as across it.">
          <input type="checkbox" aria-label="Both axes" .checked=${e.scatter.bothAxes}
            @change=${p=>this.ctx.setBrushScatter({bothAxes:p.target.checked})} />
          Both axes
        </label>
      </div>
      ${h("Count","count","How many dabs are laid at each step of the stroke.",1,sr,e.scatter.count,p=>p)}
      ${h("Count jitter","countJitter","How many dabs a step can randomly lose, as a share of the count.",0,100,Math.round(e.scatter.countJitter*100),p=>p/100)}
      `,color:()=>v`
      ${c("Hue","hue","How far each dab's hue can randomly turn, up to half a turn either way. Needs a colour that is not grey.")}
      ${c("Saturation","saturation","How much more or less colourful each dab can randomly be.")}
      ${c("Brightness","brightness","How much lighter or darker each dab can randomly be.")}
      ${c("Purity","purity","Makes the whole stroke less colourful (toward grey at -100%) or more.",-100)}
      ${c("FG/BG","fgBg","How far each dab can randomly move from the stroke colour toward the fill colour.")}
      <div class="section">
        <label title="The colour FG/BG moves each dab toward; it is also the fill colour of shapes.">Fill colour</label>
        <input type="color" aria-label="Fill colour" .value=${this.ctx.state.fillColor} @input=${p=>this._onFillColor(p)} />
      </div>
      ${l("Pressure","fgBgPressure","Let pen pressure set the mix instead of chance: light pressure goes toward the fill colour. A mouse always presses fully.")}
      ${l("Per tip","perTip","Vary the colour at every dab. Off picks one variation for the whole stroke.")}
      `,texture:()=>v`
      <div class="section">
        <label title="The paper grain that holes the paint. It stays fixed on the canvas, so a second pass over a spot shows the same grain.">Pattern</label>
        <select class="font-select" aria-label="Texture pattern" .value=${e.texture.pattern}
          @change=${p=>this.ctx.setBrushTexture({pattern:p.target.value})}>
          <option value="none" ?selected=${e.texture.pattern==="none"}>None</option>
          ${Ao().map(p=>v`<option value=${p.id} ?selected=${e.texture.pattern===p.id}>${p.label}</option>`)}
        </select>
      </div>
      ${d("Scale","scale","How large the grain is.",10,400)}
      ${d("Depth","depth","How strongly the grain holes the paint.",0,100)}
      <div class="section">
        <label title="Multiply thins the paint by the grain. Subtract takes the grain off the paint, so thin strokes break up first. Height only lets paint into the pits once the stroke is heavy.">Mode</label>
        <select class="font-select" aria-label="Texture mode" .value=${e.texture.mode}
          @change=${p=>this.ctx.setBrushTexture({mode:p.target.value})}>
          ${Object.keys(xi).map(p=>v`<option value=${p} ?selected=${e.texture.mode===p}>${xi[p]}</option>`)}
        </select>
      </div>
      ${u("Invert","invert","Swap the grain's peaks and pits.")}
      ${u("Per tip","perTip","Grain every dab on its own. Off grains the stroke as a whole, so overlapping dabs do not fill the pits in.")}
      ${e.texture.perTip?u("Depth by pressure","depthPressure","Let pen pressure scale the depth of each dab: light pressure shows less grain. A mouse always presses fully."):A}
      `,ink:()=>v`
      <div class="section">
        <label title="Fades the flow along the stroke (Photoshop's flow fade); at 100% it reaches nothing at the length below.">Depletion</label>
        <input type="range" min="0" max="100" .value=${String(Math.round(e.ink.depletion*100))}
          @input=${p=>this.ctx.setBrushInk({depletion:Number(p.target.value)/100})} />
        <span class="size-value">${Math.round(e.ink.depletion*100)}%</span>
      </div>
      ${e.ink.depletion>0?v`
        <div class="section">
          <label>Depl. Len</label>
          <input type="range" min="100" max="2000" .value=${String(e.ink.depletionLength)}
            @input=${p=>this.ctx.setBrushInk({depletionLength:Number(p.target.value)})} />
          <span class="size-value">${e.ink.depletionLength}px</span>
        </div>
      `:A}
      ${e.flow<1||e.pressureOpacity||e.dynamics.flowVelocity!==0?v`
      <div class="section">
        <label>Buildup</label>
        <input type="range" min="0" max="100" .value=${String(Math.round(e.ink.buildup*100))}
          @input=${p=>this.ctx.setBrushInk({buildup:Number(p.target.value)/100})} />
        <span class="size-value">${Math.round(e.ink.buildup*100)}%</span>
      </div>
      `:A}
      ${t!=="eraser"?v`
      <div class="section">
        <label>Wetness</label>
        <input type="range" min="0" max="100" .value=${String(Math.round(e.ink.wetness*100))}
          @input=${p=>this.ctx.setBrushInk({wetness:Number(p.target.value)/100})} />
        <span class="size-value">${Math.round(e.ink.wetness*100)}%</span>
      </div>
      `:A}
      `},f=ed.filter(p=>p.id!=="color"||t!=="eraser"),m=f.some(p=>p.id===this._brushPage)?this._brushPage:"stroke";return v`
      <div class="brush-settings">
        <div class="brush-pages" role="tablist" aria-label="Brush settings pages" aria-orientation=${this.ctx.isMobile?"horizontal":"vertical"}>
          ${f.map(p=>v`
            <button class="brush-page-btn ${p.id===m?"active":""}" role="tab" id=${`brush-tab-${p.id}`}
              aria-selected=${p.id===m?"true":"false"} aria-controls="brush-page"
              tabindex=${p.id===m?"0":"-1"}
              @click=${()=>{this._brushPage=p.id}}
              @keydown=${g=>this._onBrushTabKey(g,f.map(y=>y.id))}
            >${p.label}${sd(p.id,e)?v`<span class="page-dot" title="In use"></span>`:A}</button>
          `)}
        </div>
        <div class="brush-page" id="brush-page" role="tabpanel" aria-labelledby=${`brush-tab-${m}`}>
          ${_[m]()}
        </div>
      </div>
    `}render(){if(!this._ctx.value)return v``;const e=this.ctx.state,{strokeColor:t,fillColor:s,useFill:i,activeTool:r,stampImage:n,stampSize:a,brush:o}=e,h=this.ctx.userPresets??[],c=Je.find(f=>f.id===e.activePreset)??h.find(f=>f.id===e.activePreset),l=c??Je[0],d=e.isPresetModified||!c&&e.activePreset!=="round";if(this._brushDropdownOpen&&this.ctx.loadUserTips?.(["",...this._openGroups]),r==="select")return this._ctx.value.transformActive?this._renderTransformSettings():v`
        <div class="section" style="padding:16px;color:#888;font-size:12px;text-align:center;line-height:1.5;">
          Draw a selection to transform, or press
          <kbd style="background:#333;padding:1px 5px;border-radius:3px;font-size:11px;">${navigator.platform?.startsWith("Mac")?"⌘":"Ctrl"}+T</kbd>
          to transform the entire layer.
        </div>
      `;const u=o.size,_=this.ctx.isMobile;return v`
      ${_?"":v`
        ${this.ctx.embedded?A:v`
        <div class="section project-section">
          <div class="project-dropdown-wrap">
            <button class="project-name-btn" @click=${this._toggleProjectDropdown}>
              ${this.ctx.currentProject?.name??"Untitled"}
              <span class="dropdown-arrow">&#9662;</span>
            </button>
            ${this._projectDropdownOpen?v`
              <div class="project-dropdown">
                ${$a(this.ctx.projectList,f=>f.id,f=>v`
                  <div class="project-item ${f.id===this.ctx.currentProject?.id?"active":""}">
                    ${this._renamingProjectId===f.id?v`
                      <input
                        class="project-rename-input"
                        aria-label="Project name"
                        .value=${f.name}
                        @keydown=${m=>this._onRenameKeydown(m,f.id)}
                        @blur=${m=>this._commitRename(m,f.id)}
                      />
                    `:v`
                      <span class="project-item-name" @click=${()=>this._onSelectProject(f.id)}>
                        ${f.name}
                      </span>
                      <button
                        class="project-item-action"
                        title="Rename project"
                        aria-label=${`Rename ${f.name}`}
                        @click=${m=>this._startRename(m,f.id)}
                      >${wn.rename}</button>
                      <button
                        class="project-item-action delete"
                        title="Delete project"
                        aria-label=${`Delete ${f.name}`}
                        @click=${m=>this._onDeleteProject(m,f.id)}
                      >${wn.delete}</button>
                    `}
                  </div>
                `)}
                <div class="project-dropdown-divider"></div>
                <button class="project-new-btn" @click=${this._onNewProject}>+ New Project</button>
              </div>
            `:""}
          </div>
        </div>
        `}
        <div class="section document-size" title="Canvas size">
          ${this.ctx.state.documentWidth} \u00d7 ${this.ctx.state.documentHeight}
        </div>
        <div class="separator"></div>
      `}

      ${ce(r)?v`
        <div class="section">
          <label>Shape</label>
          <div class="shape-picker" role="group" aria-label="Shape">
            ${Vs.map(f=>v`
              <button
                class="shape-option ${r===f?"active":""}"
                data-shape=${f}
                title=${`${oe[f]} (${dr[f]})`}
                aria-label=${`Select ${oe[f]} shape`}
                aria-pressed=${r===f?"true":"false"}
                @click=${()=>this.ctx.setTool(f)}
              >${qe[f]}</button>
            `)}
          </div>
        </div>
        <div class="separator"></div>
      `:A}

      ${r!=="eraser"&&r!=="stamp"?v`
      ${_?v`
        <div class="section">
          <label>Color</label>
          ${this._renderColorControls(t)}
        </div>
      `:v`
        <div class="section panel-wrap" data-panel="color" @focusout=${this._onPanelFocusOut}>
          <button
            class="panel-trigger color-trigger ${this._openPanel==="color"?"open":""}"
            title="Color"
            aria-label="Color"
            aria-haspopup="dialog"
            aria-controls="color-panel"
            aria-expanded=${this._openPanel==="color"?"true":"false"}
            @click=${()=>this._togglePanel("color")}
          >
            <span class="color-chip" style="background:${t}"></span>
            <span class="chevron">&#9660;</span>
          </button>
          ${this._openPanel==="color"?v`
            <div id="color-panel" class="settings-panel color-panel" role="dialog" aria-label="Color">
              <div class="panel-heading">Color</div>
              ${this._renderColorControls(t)}
            </div>
          `:A}
        </div>
      `}

      <div class="separator"></div>
      `:A}

      <div class="section">
        <label>${r==="stamp"?"Stamp size":"Size"}</label>
        <input
          type="range"
          min=${r==="stamp"?Ri:1}
          max=${r==="stamp"?Di:150}
          aria-label=${r==="stamp"?"Stamp size":"Brush size"}
          .value=${String(r==="stamp"?a:u)}
          @input=${r==="stamp"?this._onStampSize:this._onBrushSize}
        />
        ${r==="stamp"?v`
              <input
                class="stamp-size-input"
                type="number"
                min=${Ri}
                max=${Di}
                step="1"
                aria-label="Stamp size in pixels"
                title="Stamp size in pixels"
                .value=${String(a)}
                @change=${this._onStampSize}
                @keydown=${this._blurOnEnter}
              />
            `:v`<span class="size-value">${u}</span>`}
      </div>

      ${r==="pencil"||r==="eraser"?v`
        <div class="separator"></div>
        <div class="section">
          <div class="brush-dropdown-wrap">
            <button class="brush-dropdown-btn" @click=${()=>this._toggleBrushDropdown()}>
              <img src=${d?this._generateCustomPreview(o,r==="eraser"):this._generatePreview(l,r==="eraser")} alt="" />
              <span>${c||!d?l.name:"Custom"}${d?" *":""}</span>
              <span class="chevron">&#9660;</span>
            </button>
            ${this._brushDropdownOpen?v`
              <div class="brush-dropdown-panel">
                ${td.map(({id:f,label:m})=>v`
                  <div class="brush-dropdown-heading">${m}</div>
                  ${Je.filter(p=>p.category===f).map(p=>this._renderPresetRow(p,!1))}
                `)}
                ${this._renderCustomPresets(h)}
              </div>
            `:A}
          </div>
        </div>
        <div class="separator"></div>
        <div class="section">
          <label>Opacity</label>
          <input type="range" aria-label="Brush opacity" min="0" max="100" .value=${String(Math.round(o.opacity*100))}
            @input=${f=>this.ctx.setBrush({opacity:Number(f.target.value)/100})} />
          <span class="size-value">${Math.round(o.opacity*100)}%</span>
        </div>
        ${_?this._renderBrushDetails():v`
          <div class="section panel-wrap" data-panel="brush" @focusout=${this._onPanelFocusOut}>
            <button
              class="panel-trigger ${this._openPanel==="brush"?"open":""}"
              title="Brush settings"
              aria-label="Brush settings"
              aria-haspopup="dialog"
              aria-controls="brush-panel"
              aria-expanded=${this._openPanel==="brush"?"true":"false"}
              @click=${()=>this._togglePanel("brush")}
            >
              ${Zl}
              <span class="brush-trigger-label">Brush settings</span>
              <span class="chevron">&#9660;</span>
            </button>
            ${this._openPanel==="brush"?v`
              <div id="brush-panel" class="settings-panel brush-panel" role="dialog" aria-label="Brush settings">
                ${this._renderBrushDetails()}
              </div>
            `:A}
          </div>
        `}
      `:A}

      ${r==="eyedropper"?v`
        <div class="section">
          <label class="checkbox-label">
            <input type="checkbox" .checked=${e.eyedropperSampleAll}
              @change=${f=>this.ctx.setEyedropperSampleAll(f.target.checked)} />
            Sample all layers
          </label>
        </div>
      `:A}

      ${this._showsShapeOptions()?v`
            <div class="separator"></div>
            <div class="section">
              <label class="checkbox-label">
                <input type="checkbox" .checked=${i} @change=${this._onUseFill} />
                Fill
              </label>
              ${i?v`
                    <input
                      type="color"
                      .value=${s}
                      @input=${this._onFillColor}
                      title="Fill color"
                    />
                  `:""}
            </div>
          `:""}

      ${r==="crop"?v`
            <div class="separator"></div>
            <div class="section">
              <label>Ratio</label>
              <select
                .value=${this.ctx.state.cropAspectRatio}
                @change=${f=>this.ctx.setCropAspectRatio(f.target.value)}
                style="background:#444;color:#ddd;border:1px solid #555;border-radius:0.25rem;padding:0.25rem 0.375rem;font-size:0.8125rem;cursor:pointer;"
              >
                <option value="free">Free</option>
                <option value="1:1">1:1</option>
                <option value="4:3">4:3</option>
                <option value="3:2">3:2</option>
                <option value="16:9">16:9</option>
                <option value="3:4">3:4</option>
                <option value="2:3">2:3</option>
                <option value="9:16">9:16</option>
              </select>
            </div>
          `:""}

      ${r==="text"?v`
            <div class="separator"></div>
            <div class="section">
              <label>Font</label>
              <select
                class="font-select"
                aria-label="Font"
                .value=${this.ctx.state.fontFamily}
                @change=${f=>this.ctx.setFontFamily(f.target.value)}
              >
                <option value="sans-serif">Sans-serif</option>
                <option value="serif">Serif</option>
                <option value="monospace">Monospace</option>
                <option value="Arial">Arial</option>
                <option value="Georgia">Georgia</option>
                <option value="Courier New">Courier New</option>
                <option value="Verdana">Verdana</option>
                <option value="Times New Roman">Times New Roman</option>
              </select>
            </div>
            <div class="section">
              <label>Size</label>
              <input
                class="font-size-input"
                type="number"
                min="8"
                max="200"
                .value=${String(this.ctx.state.fontSize)}
                @change=${f=>this.ctx.setFontSize(Number(f.target.value))}
                @keydown=${this._blurOnEnter}
              />
            </div>
            <div class="section">
              <button
                class="font-toggle ${this.ctx.state.fontBold?"active":""}"
                title="Bold"
                @click=${()=>this.ctx.setFontBold(!this.ctx.state.fontBold)}
              ><strong>B</strong></button>
              <button
                class="font-toggle ${this.ctx.state.fontItalic?"active":""}"
                title="Italic"
                @click=${()=>this.ctx.setFontItalic(!this.ctx.state.fontItalic)}
              ><em>I</em></button>
            </div>
          `:""}

      ${r==="stamp"?v`
            <div class="stamp-line">
              <button
                class="stamp-btn"
                @click=${this._uploadStamp}
                title="Upload one or more stamp images"
                aria-label="Upload one or more stamp images"
                ?disabled=${this._stampBusy}
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                  <polyline points="17 8 12 3 7 8"/>
                  <line x1="12" y1="3" x2="12" y2="15"/>
                </svg>
              </button>
              ${this._recentStamps.length>0?v`
                    <div class="stamp-row">
                      ${this._recentStamps.map((f,m)=>v`
                          <div class="stamp-thumb-wrap">
                            <button
                              class="stamp-thumb ${e.activeStampId===f.id?"active":""}"
                              aria-label=${`Select recent stamp ${m+1}`}
                              aria-pressed=${e.activeStampId===f.id?"true":"false"}
                              title=${`Select recent stamp ${m+1}`}
                              @click=${()=>this._selectStamp(f)}
                              ?disabled=${this._stampBusy}
                            >
                              <img
                                src=${this._thumbUrls.get(f.id)??""}
                                alt=""
                                loading="lazy"
                                decoding="async"
                              />
                            </button>
                            <button
                              class="stamp-delete"
                              aria-label=${`Delete recent stamp ${m+1}`}
                              title=${`Delete recent stamp ${m+1}`}
                              @click=${p=>this._deleteStamp(f,p)}
                              ?disabled=${this._stampBusy}
                            >&times;</button>
                          </div>
                        `)}
                    </div>
                  `:""}
            </div>
            <div
              class="stamp-help ${this._stampMessageError?"error":""}"
              role="status"
              aria-live="polite"
            >
              ${this._stampMessage||(n?"Click the canvas to place the selected stamp. Enter accepts; Escape cancels.":"Choose a recent stamp or upload an image, then click the canvas.")}
            </div>
          `:""}

      ${this.ctx.saving?v`
            <div class="saving-indicator" role="img" aria-label="Saving">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 12a9 9 0 1 1-6.219-8.56"></path>
              </svg>
            </div>
          `:""}

      <dialog class="new-project-dialog" @keydown=${this._onNewProjectKeydown}>
        <p class="dialog-title">New Project</p>
        <div class="dialog-field">
          <label>Name</label>
          <input
            type="text"
            class="new-project-name-input"
            .value=${this._newProjectName}
            @input=${f=>{this._newProjectName=f.target.value}}
          />
        </div>
        <div class="dialog-field">
          <label>Canvas Size</label>
          <div class="dialog-presets">
            ${Ql.map(f=>v`
              <button
                class="dialog-preset-btn ${String(f.width)===this._newProjectWidth&&String(f.height)===this._newProjectHeight?"active":""}"
                @click=${()=>this._selectNewProjectPreset(f)}
              >${f.label}</button>
            `)}
          </div>
          <div class="dialog-size-row">
            <input
              class="dialog-size-input"
              type="number"
              min="1"
              max="8192"
              .value=${this._newProjectWidth}
              @input=${f=>{this._newProjectWidth=f.target.value}}
            />
            <span>\u00d7</span>
            <input
              class="dialog-size-input"
              type="number"
              min="1"
              max="8192"
              .value=${this._newProjectHeight}
              @input=${f=>{this._newProjectHeight=f.target.value}}
            />
          </div>
        </div>
        <div class="dialog-actions">
          <button class="dialog-cancel-btn" @click=${this._cancelNewProject}>Cancel</button>
          <button class="dialog-create-btn" @click=${this._confirmNewProject}>Create</button>
        </div>
      </dialog>
    `}};rt.styles=le`
    :host {
      display: flex;
      align-items: center;
      background: #333;
      /* Right padding keeps controls clear of the saving indicator */
      padding: 0.375rem 2.75rem 0.375rem 1rem;
      column-gap: 1rem;
      row-gap: 0.25rem;
      color: #ddd;
      font-family: system-ui, sans-serif;
      font-size: 0.8125rem;
      flex-wrap: wrap;
      min-height: 2.75rem;
      position: relative;
    }

    .section {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }

    label {
      color: #aaa;
      white-space: nowrap;
    }

    .color-grid {
      display: flex;
      flex-wrap: wrap;
      gap: 0.1875rem;
      max-width: 15rem;
    }

    .color-swatch {
      width: 1.5rem;
      height: 1.5rem;
      border-radius: 0.1875rem;
      border: 0.125rem solid transparent;
      cursor: pointer;
      padding: 0;
      box-sizing: border-box;
    }

    .color-swatch:hover {
      border-color: #888;
    }

    .color-swatch.active {
      border-color: #5b8cf7;
    }

    input[type="color"] {
      width: 1.75rem;
      height: 1.75rem;
      border: none;
      border-radius: 0.25rem;
      padding: 0;
      cursor: pointer;
      background: none;
    }

    input[type="range"] {
      width: 6.25rem;
      accent-color: #5b8cf7;
    }

    .size-value {
      min-width: 1.5rem;
      text-align: center;
    }

    .stamp-size-input {
      width: 4.5rem;
      min-height: 2rem;
      box-sizing: border-box;
      border: 1px solid #555;
      border-radius: 0.25rem;
      background: #222;
      color: #ddd;
      padding: 0.25rem 0.375rem;
    }

    .checkbox-label {
      display: flex;
      align-items: center;
      gap: 0.25rem;
      cursor: pointer;
      color: #aaa;
    }

    .checkbox-label input {
      accent-color: #5b8cf7;
    }

    .stamp-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      background: #5b8cf7;
      color: white;
      border: none;
      border-radius: 6px;
      cursor: pointer;
      padding: 6px;
      flex-shrink: 0;
    }

    .stamp-btn svg {
      width: 15px;
      height: 15px;
    }

    .stamp-btn:hover {
      background: #4a7be6;
    }

    .stamp-btn:disabled,
    .stamp-thumb:disabled,
    .stamp-delete:disabled {
      opacity: 0.55;
      cursor: wait;
    }

    .separator {
      width: 0.0625rem;
      height: 1.5rem;
      background: #555;
    }

    .stamp-row {
      display: flex;
      gap: 0.25rem;
      overflow-x: auto;
      overflow-y: hidden;
      padding: 0.125rem 0;
      align-items: center;
      scrollbar-width: none;
    }

    .stamp-row::-webkit-scrollbar {
      display: none;
    }

    .stamp-thumb-wrap {
      position: relative;
      flex-shrink: 0;
      overflow: hidden;
      border-radius: 0.25rem;
    }

    .stamp-thumb {
      width: 2.75rem;
      height: 2.75rem;
      border-radius: 0.25rem;
      border: 0.125rem solid transparent;
      object-fit: contain;
      background: #222;
      cursor: pointer;
      display: block;
      padding: 0;
      box-sizing: border-box;
    }

    .stamp-thumb img {
      width: 100%;
      height: 100%;
      object-fit: contain;
      display: block;
    }

    .stamp-thumb:hover {
      border-color: #888;
    }

    .stamp-thumb.active {
      border-color: #5b8cf7;
    }

    .stamp-delete {
      position: absolute;
      top: -0.125rem;
      right: -0.125rem;
      width: 1.5rem;
      height: 1.5rem;
      border-radius: 50%;
      background: #444;
      color: #fff;
      border: none;
      font-size: 0.875rem;
      line-height: 1.5rem;
      text-align: center;
      cursor: pointer;
      padding: 0;
      z-index: 1;
    }

    @media (hover: hover) and (pointer: fine) {
      .stamp-delete {
        opacity: 0;
        pointer-events: none;
      }

      .stamp-thumb-wrap:hover .stamp-delete,
      .stamp-thumb-wrap:focus-within .stamp-delete {
        opacity: 1;
        pointer-events: auto;
      }
    }

    @media (hover: none), (pointer: coarse) {
      .stamp-btn {
        width: 2.75rem;
        height: 2.75rem;
      }

      .stamp-thumb-wrap {
        display: flex;
        gap: 0.125rem;
        overflow: visible;
      }

      .stamp-delete {
        position: static;
        width: 2.75rem;
        height: 2.75rem;
        border-radius: 0.25rem;
        line-height: 2.75rem;
      }
    }

    .stamp-delete:hover {
      background: #e55;
    }

    .stamp-line {
      flex-basis: 100%;
      display: flex;
      align-items: center;
      gap: 0.5rem;
      margin-left: -0.5rem;
    }

    .stamp-help {
      flex-basis: 100%;
      color: #aaa;
      font-size: 0.75rem;
      margin-left: -0.5rem;
    }

    .stamp-help.error {
      color: #ff8a8a;
    }

    .project-section {
      position: relative;
    }

    .project-dropdown-wrap {
      position: relative;
    }

    .project-name-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.5rem;
      cursor: pointer;
      font-size: 0.8125rem;
      display: flex;
      align-items: center;
      gap: 0.25rem;
      max-width: 12rem;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .project-name-btn:hover {
      background: #555;
    }

    .dropdown-arrow {
      font-size: 0.625rem;
      opacity: 0.7;
    }

    .project-dropdown {
      position: absolute;
      top: 100%;
      left: 0;
      margin-top: 0.25rem;
      background: #3a3a3a;
      border: 1px solid #555;
      border-radius: 0.375rem;
      min-width: 14rem;
      max-height: 20rem;
      overflow-y: auto;
      z-index: 100;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
      padding: 0.25rem 0;
    }

    .project-item {
      display: flex;
      align-items: center;
      padding: 0.375rem 0.5rem;
      gap: 0.25rem;
    }

    .project-item.active {
      background: #4a4a4a;
    }

    .project-item-name {
      flex: 1;
      cursor: pointer;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      padding: 0.125rem 0;
    }

    .project-item-name:hover {
      color: #fff;
    }

    .project-item-action {
      background: none;
      border: none;
      color: #bbb;
      cursor: pointer;
      padding: 0;
      border-radius: 0.25rem;
      line-height: 0;
      width: 1.75rem;
      height: 1.75rem;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      flex-shrink: 0;
    }

    .project-item-action:hover,
    .project-item-action:focus-visible {
      color: #fff;
      background: #555;
    }

    .project-item-action.delete:hover,
    .project-item-action.delete:focus-visible {
      color: #ff8080;
      background: #5a3434;
    }

    .project-rename-input {
      flex: 1;
      background: #2a2a2a;
      border: 1px solid #5b8cf7;
      border-radius: 0.1875rem;
      color: #ddd;
      padding: 0.125rem 0.25rem;
      font-size: 0.8125rem;
      outline: 2px solid transparent;
    }

    .project-dropdown-divider {
      height: 1px;
      background: #555;
      margin: 0.25rem 0;
    }

    .project-new-btn {
      display: block;
      width: 100%;
      text-align: left;
      background: none;
      border: none;
      color: #5b8cf7;
      cursor: pointer;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      height: auto;
      border-radius: 0;
    }

    .project-new-btn:hover {
      background: #4a4a4a;
      color: #7aa8ff;
    }

    .saving-indicator {
      position: absolute;
      top: 0.75rem;
      right: 1rem;
      color: #888;
      width: 1.25rem;
      height: 1.25rem;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .saving-indicator svg {
      width: 100%;
      height: 100%;
      animation: spin 1s linear infinite;
    }

    @keyframes spin {
      100% { transform: rotate(360deg); }
    }

    dialog {
      background: #3a3a3a;
      border: 1px solid #555;
      border-radius: 0.5rem;
      color: #ddd;
      padding: 1.25rem;
      min-width: 18rem;
      box-shadow: 0 8px 24px rgba(0,0,0,0.5);
      font-family: system-ui, sans-serif;
      font-size: 0.8125rem;
    }

    dialog::backdrop {
      background: rgba(0,0,0,0.5);
    }

    .dialog-title {
      font-size: 1rem;
      font-weight: 600;
      margin: 0 0 1rem 0;
    }

    .dialog-field {
      display: flex;
      flex-direction: column;
      gap: 0.25rem;
      margin-bottom: 0.75rem;
    }

    .dialog-field label {
      color: #aaa;
      font-size: 0.75rem;
    }

    .dialog-field input[type="text"] {
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      outline: 2px solid transparent;
    }

    .dialog-field input[type="text"]:focus {
      border-color: #5b8cf7;
    }

    .dialog-presets {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
      margin-bottom: 0.5rem;
    }

    .dialog-preset-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.5rem;
      cursor: pointer;
      font-size: 0.75rem;
      height: auto;
    }

    .dialog-preset-btn:hover {
      background: #555;
    }

    .dialog-preset-btn.active {
      border-color: #5b8cf7;
      color: #5b8cf7;
    }

    .dialog-size-row {
      display: flex;
      align-items: center;
      gap: 0.375rem;
    }

    .dialog-size-input {
      width: 5rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      text-align: center;
      outline: 2px solid transparent;
    }

    .dialog-size-input:focus {
      border-color: #5b8cf7;
    }

    .dialog-actions {
      display: flex;
      justify-content: flex-end;
      gap: 0.5rem;
      margin-top: 1rem;
    }

    .dialog-cancel-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.375rem 0.75rem;
      cursor: pointer;
      font-size: 0.8125rem;
    }

    .dialog-cancel-btn:hover {
      background: #555;
    }

    .dialog-create-btn {
      background: #5b8cf7;
      color: white;
      border: none;
      border-radius: 0.25rem;
      padding: 0.375rem 0.75rem;
      cursor: pointer;
      font-size: 0.8125rem;
    }

    .dialog-create-btn:hover {
      background: #4a7be6;
    }

    .font-select {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.375rem;
      font-size: 0.8125rem;
      cursor: pointer;
    }

    .font-size-input {
      width: 3.5rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.375rem;
      font-size: 0.8125rem;
      text-align: center;
    }

    .font-toggle {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.75rem;
      height: 1.75rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      cursor: pointer;
      font-size: 0.8125rem;
      padding: 0;
    }

    .font-toggle.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    /* Brush preset dropdown */
    .brush-dropdown-wrap {
      position: relative;
      width: 100%;
    }

    .brush-dropdown-btn {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      width: 100%;
      padding: 0.25rem 0.5rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.375rem;
      cursor: pointer;
      color: #ddd;
      font-size: 0.8125rem;
    }

    .brush-dropdown-btn:hover {
      border-color: #888;
    }

    .brush-dropdown-btn img {
      width: 80px;
      height: 24px;
      border-radius: 0.125rem;
      object-fit: cover;
    }

    .brush-dropdown-btn .chevron {
      margin-left: auto;
      font-size: 0.625rem;
      color: #888;
    }

    /* As wide as a preview and a full name need, not as narrow as the button. */
    .brush-dropdown-panel {
      position: absolute;
      top: 100%;
      left: 0;
      width: 22.5rem;
      max-width: calc(100vw - 1rem);
      box-sizing: border-box;
      z-index: 50;
      background: #333;
      border: 1px solid #555;
      border-radius: 0.375rem;
      margin-top: 0.125rem;
      max-height: min(30rem, calc(100vh - 6rem));
      overflow-y: auto;
      overscroll-behavior: contain;
      padding: 0.25rem 0;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
    }

    :host([mobile]) .brush-dropdown-panel {
      width: auto;
      right: 0;
    }

    .brush-dropdown-item {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      width: 100%;
      padding: 0.375rem 0.5rem;
      background: none;
      border: none;
      cursor: pointer;
      color: #ccc;
      font-size: 0.8125rem;
      text-align: left;
    }

    .brush-dropdown-item span {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .brush-dropdown-row {
      display: flex;
      align-items: center;
    }
    .brush-dropdown-row .brush-dropdown-item {
      flex: 1;
      min-width: 0;
    }
    .brush-dropdown-delete {
      background: none;
      border: none;
      color: #999;
      cursor: pointer;
      padding: 0.25rem 0.5rem;
      font-size: 1rem;
      line-height: 1;
    }
    .brush-dropdown-delete:hover {
      color: #f88;
    }
    .brush-dropdown-heading {
      padding: 0.375rem 0.5rem 0.25rem;
      font-size: 0.6875rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      color: #999;
      text-transform: uppercase;
      border-top: 1px solid #444;
    }

    .brush-dropdown-heading:first-child {
      border-top: none;
    }

    /* Imported groups are named after their files, so keep the name as typed. */
    .brush-dropdown-heading.group-heading .group-toggle {
      text-transform: none;
      letter-spacing: normal;
      font-weight: normal;
      font-size: 0.75rem;
    }
    .brush-dropdown-heading.group-heading {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0;
    }
    .group-toggle {
      flex: 1;
      min-width: 0;
      background: none;
      border: none;
      color: inherit;
      font: inherit;
      text-transform: inherit;
      text-align: left;
      padding: 0.25rem 0.5rem;
      cursor: pointer;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .import-progress {
      width: 100%;
      height: 0.5rem;
    }
    .tip-grid {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
      max-height: 7.5rem;
      overflow-y: auto;
    }
    .tip-cell {
      position: relative;
    }
    .tip-cell > button.tip-pick {
      display: block;
      width: 2.5rem;
      height: 2.5rem;
      padding: 0;
      background: #fff;
      border: 2px solid #555;
      border-radius: 0.25rem;
      cursor: pointer;
    }
    .tip-cell > button.tip-pick.active {
      border-color: #7ab8ff;
    }
    .tip-cell > button.tip-pick img {
      width: 100%;
      height: 100%;
      object-fit: contain;
      display: block;
    }
    .tip-cell > button.tip-delete {
      position: absolute;
      top: -0.25rem;
      right: -0.25rem;
      width: 1rem;
      height: 1rem;
      padding: 0;
      border-radius: 50%;
      background: #222;
      color: #ddd;
      border: 1px solid #666;
      font-size: 0.75rem;
      line-height: 1;
      cursor: pointer;
    }
    .tip-message {
      font-size: 0.75rem;
      color: #bbb;
    }
    .tip-message.error {
      color: #f99;
    }
    .brush-dropdown-item:hover {
      background: #444;
    }

    .brush-dropdown-item.active {
      background: #3a3a4a;
      color: #5b8cf7;
    }

    .brush-dropdown-item img {
      flex: none;
      width: 120px;
      height: 36px;
      border-radius: 0.25rem;
      object-fit: cover;
      overflow: hidden;
    }

    .brush-dropdown-item img.tip-thumb {
      width: 56px;
    }

    /* Pill-button shape selector */
    .pill-row {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
    }

    .pill-btn {
      background: #444;
      color: #bbb;
      border: 1px solid #555;
      border-radius: 1rem;
      padding: 0.15rem 0.5rem;
      cursor: pointer;
      font-size: 0.75rem;
      white-space: nowrap;
      height: auto;
    }

    .pill-btn:hover {
      background: #555;
      color: #ddd;
    }

    .pill-btn.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    .shape-picker {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
    }

    .shape-option {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 2rem;
      height: 2rem;
      padding: 0.35rem;
      border: 1px solid #555;
      border-radius: 0.375rem;
      background: #444;
      color: #bbb;
      cursor: pointer;
    }

    .shape-option:hover {
      background: #555;
      color: #fff;
    }

    .shape-option.active {
      background: #5b8cf7;
      border-color: #77a1ff;
      color: #fff;
    }

    .shape-option svg {
      width: 1.125rem;
      height: 1.125rem;
    }

    /* Dimmed control */

    /* Transform numeric panel */
    .transform-section {
      display: flex;
      flex-direction: column;
      gap: 0.2rem;
    }

    .transform-section > label {
      font-size: 0.6875rem;
      color: #888;
      white-space: nowrap;
    }

    .transform-row {
      display: flex;
      align-items: center;
      gap: 0.25rem;
    }

    .transform-input {
      width: 4rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.2rem 0.3rem;
      font-size: 0.75rem;
      text-align: center;
      outline: 2px solid transparent;
      -moz-appearance: textfield;
    }

    .transform-input::-webkit-inner-spin-button,
    .transform-input::-webkit-outer-spin-button {
      -webkit-appearance: none;
      margin: 0;
    }

    .transform-input:focus {
      border-color: #5b8cf7;
    }

    .transform-suffix {
      color: #888;
      font-size: 0.75rem;
    }

    .flip-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.75rem;
      height: 1.75rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      cursor: pointer;
      font-size: 0.75rem;
      padding: 0;
    }

    .flip-btn:hover {
      background: #555;
    }

    .flip-btn.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    .aspect-lock-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.25rem;
      height: 1.25rem;
      background: none;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #888;
      cursor: pointer;
      padding: 0;
      font-size: 0.6875rem;
      flex-shrink: 0;
    }

    .aspect-lock-btn:hover {
      border-color: #888;
      color: #ddd;
    }

    .aspect-lock-btn.active {
      border-color: #5b8cf7;
      color: #5b8cf7;
    }

    /* ── Desktop settings panels (color, brush) ── */
    .panel-wrap {
      position: relative;
    }

    .panel-trigger {
      display: flex;
      align-items: center;
      gap: 0.375rem;
      height: 2rem;
      padding: 0 0.5rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.375rem;
      color: #ddd;
      cursor: pointer;
      font-size: 0.8125rem;
      white-space: nowrap;
    }

    .panel-trigger:hover,
    .panel-trigger.open {
      border-color: #888;
    }

    .panel-trigger:focus-visible,
    .brush-page-btn:focus-visible {
      outline: 2px solid #5b8cf7;
      outline-offset: 1px;
    }

    .panel-trigger .chevron {
      font-size: 0.625rem;
      color: #888;
    }

    .color-chip {
      width: 1.25rem;
      height: 1.25rem;
      border-radius: 0.1875rem;
      box-shadow: inset 0 0 0 1px rgba(255,255,255,0.25);
    }

    .settings-panel {
      position: absolute;
      top: 100%;
      left: 0;
      margin-top: 0.25rem;
      z-index: 100;
      background: #3a3a3a;
      border: 1px solid #555;
      border-radius: 0.5rem;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
      padding: 0.75rem;
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      gap: 0.5rem;
    }

    .color-panel .color-grid {
      /* Nine swatches per row, matching the two-row palette. */
      width: calc(9 * 1.5rem + 8 * 0.1875rem);
      max-width: none;
    }

    .custom-color {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      cursor: pointer;
    }

    .custom-color:hover {
      color: #ddd;
    }

    .brush-panel {
      width: 34rem;
      max-width: calc(100vw - 1rem);
      max-height: calc(100vh - 6rem);
      overflow-y: auto;
      overscroll-behavior: contain;
    }

    .panel-heading {
      color: #999;
      font-size: 0.6875rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      margin-top: 0.25rem;
    }

    .panel-heading:first-child {
      margin-top: 0;
    }

    /* Slider rows line up as label | slider | value columns. */
    .brush-panel .section {
      display: grid;
      grid-template-columns: 5.5rem minmax(0, 1fr) 3rem;
      align-items: center;
      gap: 0.5rem;
    }

    .brush-panel .section > input[type="range"] {
      width: 100%;
    }

    .brush-panel .section > .size-value {
      text-align: right;
    }

    .brush-panel .section > .checkbox-label {
      grid-column: 1 / -1;
    }

    .brush-panel .section > select {
      grid-column: 2 / -1;
      justify-self: start;
    }

    .brush-panel .section > .pill-row {
      grid-column: 2 / -1;
    }

    /* Brush settings: a column of page tabs beside the page on show. */
    .brush-settings {
      display: grid;
      grid-template-columns: 6.5rem minmax(0, 1fr);
      gap: 0.75rem;
      min-height: 17rem;
    }

    .brush-pages {
      display: flex;
      flex-direction: column;
      gap: 0.125rem;
      padding-right: 0.5rem;
      border-right: 1px solid #4a4a4a;
    }

    .brush-page-btn {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 0.25rem;
      background: none;
      border: none;
      border-radius: 0.25rem;
      padding: 0.375rem 0.5rem;
      color: #aaa;
      cursor: pointer;
      font-size: 0.8125rem;
      text-align: left;
      white-space: nowrap;
    }

    .brush-page-btn:hover {
      background: #444;
      color: #ddd;
    }

    .brush-page-btn.active {
      background: #5b8cf7;
      color: #fff;
    }

    .page-dot {
      flex: none;
      width: 0.375rem;
      height: 0.375rem;
      border-radius: 50%;
      background: #5b8cf7;
    }

    .brush-page-btn.active .page-dot {
      background: #fff;
    }

    .brush-page {
      display: flex;
      flex-direction: column;
      gap: 0.5rem;
      min-width: 0;
    }

    /* Narrower desktop windows: keep the bar on one row. */
    @media (max-width: 1180px) {
      :host(:not([mobile])) {
        column-gap: 0.75rem;
      }

      :host(:not([mobile])) input[type="range"] {
        width: 5rem;
      }

      :host(:not([mobile])) .brush-panel input[type="range"] {
        width: 100%;
      }

      :host(:not([mobile])) .brush-trigger-label {
        display: none;
      }
    }

    .document-size {
      color: #aaa;
      font-size: 0.75rem;
      white-space: nowrap;
    }

    /* ── Inside mobile popover ─────────────────── */
    :host([mobile]) {
      flex-direction: column;
      align-items: flex-start;
      padding: 0;
      min-height: 0;
      background: transparent;
      touch-action: manipulation;
    }

    :host([mobile]) .section {
      width: 100%;
      min-width: 0;
    }

    :host([mobile]) .brush-settings {
      grid-template-columns: minmax(0, 1fr);
      width: 100%;
      min-height: 0;
    }

    :host([mobile]) .brush-pages {
      flex-direction: row;
      overflow-x: auto;
      scrollbar-width: none;
      padding: 0 0 0.375rem;
      border-right: none;
      border-bottom: 1px solid #4a4a4a;
    }

    :host([mobile]) .brush-page-btn {
      min-height: 2.75rem;
      padding: 0 0.75rem;
    }

    :host([mobile]) .separator {
      display: none;
    }

    :host([mobile]) input[type="range"] {
      flex: 1;
      min-width: 0;
      width: 0;
    }

    :host([mobile]) .stamp-size-input {
      min-height: 2.75rem;
    }
  `;ut([T()],rt.prototype,"_aspectLock",2);ut([T()],rt.prototype,"_recentStamps",2);ut([T()],rt.prototype,"_stampBusy",2);ut([T()],rt.prototype,"_stampMessage",2);ut([T()],rt.prototype,"_stampMessageError",2);ut([T()],rt.prototype,"_projectDropdownOpen",2);ut([T()],rt.prototype,"_brushPage",2);ut([T()],rt.prototype,"_brushDropdownOpen",2);ut([T()],rt.prototype,"_tipMessage",2);ut([T()],rt.prototype,"_tipMessageError",2);ut([T()],rt.prototype,"_openGroups",2);ut([T()],rt.prototype,"_importProgress",2);ut([T()],rt.prototype,"_openPanel",2);ut([T()],rt.prototype,"_renamingProjectId",2);ut([T()],rt.prototype,"_newProjectName",2);ut([T()],rt.prototype,"_newProjectWidth",2);ut([T()],rt.prototype,"_newProjectHeight",2);rt=ut([de("tool-settings")],rt);var id=Object.defineProperty,rd=Object.getOwnPropertyDescriptor,ur=(e,t,s,i)=>{for(var r=i>1?void 0:i?rd(t,s):t,n=e.length-1,a;n>=0;n--)(a=e[n])&&(r=(i?a(t,s,r):a(r))||r);return i&&r&&id(t,s,r),r};const xn=typeof navigator<"u"&&/Mac|iPhone|iPad/.test(navigator.platform)?"⌘":"Ctrl+",Ms=[...Vs],Re=[["select","move","crop","hand"],["pencil","eraser"],Ms,["fill","stamp","text","eyedropper"]],nd=[{hex:"#000000",name:"Black"},{hex:"#ffffff",name:"White"},{hex:"#ff3b30",name:"Red"},{hex:"#ff9500",name:"Orange"},{hex:"#ffcc00",name:"Yellow"},{hex:"#34c759",name:"Green"},{hex:"#00c7be",name:"Teal"},{hex:"#007aff",name:"Blue"},{hex:"#5856d6",name:"Indigo"},{hex:"#af52de",name:"Purple"},{hex:"#ff2d55",name:"Pink"},{hex:"#a2845e",name:"Brown"}],fi=[{label:"S",value:4},{label:"M",value:16},{label:"L",value:40}];let ns=class extends Bt{constructor(){super(...arguments),this._popoverGroup=null,this._isFullscreen=!1,this._lastToolPerGroup=new Map,this._ctx=new Me(this,{context:Fe,subscribe:!0}),this._onFullscreenChange=()=>{this._isFullscreen=!!document.fullscreenElement},this._toggleFullscreen=()=>{document.fullscreenElement?document.exitFullscreen():document.documentElement.requestFullscreen(),this._closePopover()},this._onPopoverEscape=e=>{e.key!=="Escape"||this._popoverGroup===null||!Ds(this,e)||(e.stopPropagation(),e.preventDefault(),this._closePopover())}}get ctx(){return this._ctx.value}get _saveLabel(){return this.ctx.embedded?"Save":"Export as PNG"}connectedCallback(){super.connectedCallback(),document.addEventListener("fullscreenchange",this._onFullscreenChange),document.addEventListener("keydown",this._onPopoverEscape,!0)}disconnectedCallback(){super.disconnectedCallback(),document.removeEventListener("fullscreenchange",this._onFullscreenChange),document.removeEventListener("keydown",this._onPopoverEscape,!0)}willUpdate(){this.toggleAttribute("mobile",this.ctx?.isMobile??!1),this.toggleAttribute("child-mode",this.ctx?.state?.childMode??!1);const e=this.ctx?.state?.activeTool;if(e){const t=Re.findIndex(s=>s.includes(e));t!==-1&&this._lastToolPerGroup.set(t,e)}this.ctx?.state?.layersPanelOpen&&this._popoverGroup!==null&&(this._popoverGroup=null)}_selectTool(e){this.ctx.setTool(e)}render(){if(!this._ctx.value)return v``;const{activeTool:e}=this.ctx.state;return this.ctx.isMobile?this._renderMobile(e):v`
      ${Re.map((t,s)=>v`
          ${s>0?v`<div class="separator"></div>`:""}
          <div class="group">
            ${t===Ms?v`
                <button
                  class=${ce(e)?"active":""}
                  title="Shapes (U)"
                  aria-label="Shapes"
                  aria-pressed=${ce(e)}
                  @click=${()=>this._selectTool(ce(e)?e:this._lastToolPerGroup.get(s)??Vs[0])}
                >
                  ${Gr}
                </button>
              `:t.map(i=>v`
                <button
                  class=${e===i?"active":""}
                  title=${`${oe[i]} (${dr[i]})`}
                  aria-label=${oe[i]}
                  aria-pressed=${e===i}
                  @click=${()=>this._selectTool(i)}
                >
                  ${qe[i]}
                </button>
              `)}
          </div>
        `)}

      <div class="action-group">
        <div class="separator"></div>
        <button
          title=${`Undo (${xn}Z)`}
          aria-label="Undo"
          ?disabled=${!this.ctx.canUndo}
          @click=${()=>this.ctx.undo()}
        >
          ${Tt.undo}
        </button>
        <button
          title=${`Redo (${xn}Shift+Z)`}
          aria-label="Redo"
          ?disabled=${!this.ctx.canRedo}
          @click=${()=>this.ctx.redo()}
        >
          ${Tt.redo}
        </button>
        <button title=${this._saveLabel} aria-label=${this._saveLabel} @click=${()=>this.ctx.saveCanvas()}>
          ${Tt.save}
        </button>
        <button title="Clear layer" aria-label="Clear layer" @click=${()=>this.ctx.clearCanvas()}>
          ${Tt.clear}
        </button>
      </div>
    `}_closestChildSize(e){let t=fi[0].value,s=Math.abs(e-t);for(const i of fi){const r=Math.abs(e-i.value);r<s&&(s=r,t=i.value)}return t}async _confirmClearCanvas(){await this.ctx.confirm({message:"Clear the whole drawing?",confirmLabel:"Clear",danger:!0})&&this.ctx.clearCanvas(!0)}_renderChildMode(e){const t=this.ctx.state.strokeColor,s=this.ctx.state.brush.size,i=this._closestChildSize(s);return v`
      <div class="child-bar">
        <div class="child-colors">
          ${nd.map(r=>v`
            <button
              class="child-color-btn ${t===r.hex?"active":""}"
              style="background:${r.hex}${r.hex==="#ffffff"?";box-shadow:inset 0 0 0 1px #666":""}"
              aria-label=${r.name}
              @click=${()=>this.ctx.setStrokeColor(r.hex)}
            ></button>
          `)}
          <input
            type="color"
            class="child-color-picker"
            .value=${t}
            @input=${r=>this.ctx.setStrokeColor(r.target.value)}
            title="Pick color"
          />
        </div>

        <div class="child-tools-row">
          <button
            class="child-tool-btn"
            title="Undo"
            ?disabled=${!this.ctx.canUndo}
            @click=${()=>this.ctx.undo()}
          >${Tt.undo}</button>
          <button
            class="child-tool-btn"
            title="Redo"
            ?disabled=${!this.ctx.canRedo}
            @click=${()=>this.ctx.redo()}
          >${Tt.redo}</button>

          <div class="child-sep"></div>

          ${Pa.map(r=>v`
            <button
              class="child-tool-btn ${e===r?"active":""}"
              title=${oe[r]}
              @click=${()=>this._selectTool(r)}
            >${qe[r]}</button>
          `)}

          <div class="child-sep"></div>

          ${fi.map(r=>v`
            <button
              class="child-size-btn ${i===r.value?"active":""}"
              title="${r.label} brush"
              @click=${()=>this.ctx.setBrushSize(r.value)}
            >${r.label}</button>
          `)}

          <div class="child-sep"></div>

          <button
            class="child-tool-btn"
            title=${this._saveLabel}
            aria-label=${this._saveLabel}
            @click=${()=>this.ctx.saveCanvas()}
          >${Tt.save}</button>
          <button
            class="child-tool-btn"
            title="Clear canvas"
            @click=${()=>this._confirmClearCanvas()}
          >${Tt.clear}</button>
          <button
            class="child-tool-btn"
            title="Exit Child Mode"
            @click=${()=>this.ctx.setChildMode(!1)}
          >${Tt.exitChildMode}</button>
        </div>
      </div>
    `}_renderMobile(e){return this.ctx.state.childMode?this._renderChildMode(e):v`
      <!-- Undo/Redo at left -->
      <button
        title="Undo"
        ?disabled=${!this.ctx.canUndo}
        @click=${()=>this.ctx.undo()}
      >${Tt.undo}</button>
      <button
        title="Redo"
        ?disabled=${!this.ctx.canRedo}
        @click=${()=>this.ctx.redo()}
      >${Tt.redo}</button>

      <div class="separator"></div>

      <!-- Tool groups: show one representative button per group -->
      ${Re.map((t,s)=>{const r=t.find(o=>o===e)??t[0],n=t.includes(e),a=t===Ms;return v`
          <button
            class=${n?"active":""}
            title=${a?"Shapes":oe[r]}
            aria-label=${a?"Shapes":oe[r]}
            aria-expanded=${this._popoverGroup===s}
            aria-controls="mobile-tool-popover"
            @click=${()=>this._onMobileToolTap(t,s)}
          >${a?Gr:qe[r]}</button>
        `})}

      <div class="separator"></div>

      <!-- Layers button -->
      <button
        title="Layers"
        @click=${()=>{this._closePopover(),this.ctx.toggleLayersPanel()}}
      >${v`<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>`}</button>

      <!-- More actions -->
      <button
        title="More"
        @click=${()=>this._onMobileMoreTap()}
      ><svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><circle cx="12" cy="5" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="19" r="2"/></svg></button>

      <!-- Popover -->
      ${this._popoverGroup!==null?v`
        <div class="popover-backdrop" @click=${()=>this._closePopover()}></div>
        <div class="popover" id="mobile-tool-popover">
          ${this._renderPopoverContent(e)}
        </div>
      `:""}
    `}_onMobileToolTap(e,t){const{activeTool:s}=this.ctx.state;if(e.includes(s)){const r=this._popoverGroup===t?null:t;this._popoverGroup=r,r!==null&&this.ctx.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}else this.ctx.setTool(this._lastToolPerGroup.get(t)??e[0]),this._popoverGroup=null}_onMobileMoreTap(){const e=this._popoverGroup===-1?null:-1;this._popoverGroup=e,e!==null&&this.ctx.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}_closePopover(){this._popoverGroup=null}_stopCropActionKeydown(e){e.key==="Enter"&&e.stopPropagation()}_dispatchCropShortcut(e){this._closePopover(),this.dispatchEvent(new KeyboardEvent("keydown",{key:e,bubbles:!0,composed:!0}))}_renderPopoverContent(e){if(this._popoverGroup===-1)return v`
        <button class="menu-btn" @click=${()=>{const s=Re.findIndex(i=>i.includes(e));s!==-1&&this._onMobileToolTap(Re[s],s)}}>Tool settings</button>
        <div class="popover-divider"></div>
        ${this.ctx.embedded?"":v`
        <span class="popover-label">Projects</span>
        <div class="project-list">
          ${this.ctx.projectList.map(s=>v`
            <button
              class="project-item ${s.id===this.ctx.currentProject?.id?"current":""}"
              @click=${()=>{this.ctx.switchProject(s.id),this._closePopover()}}
            >
              <span class="check">${s.id===this.ctx.currentProject?.id?"✓":""}</span>
              ${s.name}
            </button>
          `)}
        </div>
        <button class="menu-btn" @click=${()=>{this.ctx.createProject("Untitled",800,600),this._closePopover()}}>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          New Project
        </button>
        <div class="popover-divider"></div>
        `}
        <button class="menu-btn" @click=${()=>{this.ctx.saveCanvas(),this._closePopover()}}>${Tt.save} ${this._saveLabel}</button>
        <button class="menu-btn" @click=${()=>{this.ctx.clearCanvas(),this._closePopover()}}>${Tt.clear} Clear layer</button>
        <div class="popover-divider"></div>
        <button class="menu-btn" @click=${this._toggleFullscreen}>
          ${this._isFullscreen?v`<svg viewBox="0 0 16 16" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M5 2v3H2M14 5h-3V2M11 14v-3h3M2 11h3v3"/></svg>`:v`<svg viewBox="0 0 16 16" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4"/></svg>`}
          ${this._isFullscreen?"Exit Fullscreen":"Fullscreen"}
        </button>
        <div class="popover-divider"></div>
        <button class="menu-btn" @click=${()=>{this.ctx.setChildMode(!0),this._closePopover()}}>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M8 14s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
          Child Mode
        </button>
      `;const t=Re[this._popoverGroup];return t?v`
      ${t.length>1&&t!==Ms?v`
        <div class="sub-tools">
          ${t.map(s=>v`
            <button
              class=${e===s?"active":""}
              title=${oe[s]}
              @click=${()=>{this.ctx.setTool(s),this._lastToolPerGroup.set(this._popoverGroup,s)}}
            >${qe[s]}</button>
          `)}
        </div>
      `:""}
      ${e==="crop"?v`
        <button
          class="menu-btn"
          @keydown=${this._stopCropActionKeydown}
          @click=${()=>this._dispatchCropShortcut("Enter")}
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
          Apply crop
        </button>
        <button
          class="menu-btn"
          @keydown=${this._stopCropActionKeydown}
          @click=${()=>this._dispatchCropShortcut("Escape")}
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>
          Cancel crop
        </button>
        <div class="popover-divider"></div>
      `:""}
      <tool-settings></tool-settings>
    `:v``}};ns.styles=le`
    :host {
      display: flex;
      flex-direction: column;
      background: #2c2c2c;
      padding: 8px;
      gap: 4px;
      width: 60px;
      box-sizing: border-box;
      overflow-x: hidden;
      overflow-y: auto;
      scrollbar-width: thin;
      scrollbar-color: #555 transparent;
    }

    .group {
      display: flex;
      flex-direction: column;
      gap: 2px;
    }

    .separator {
      height: 1px;
      background: #555;
      margin: 4px 2px;
    }

    button {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      padding: 10px;
      transition: all 0.15s ease;
    }

    button:hover {
      background: #444;
      color: #fff;
    }

    button.active {
      background: #5b8cf7;
      color: #fff;
    }

    button:disabled {
      opacity: 0.3;
      cursor: default;
    }

    button:disabled:hover {
      background: transparent;
      color: #bbb;
    }

    button svg {
      width: 20px;
      height: 20px;
    }

    .action-group {
      margin-top: auto;
      display: flex;
      flex-direction: column;
      gap: 2px;
    }

    /* ── Mobile bottom bar ─────────────────────── */
    :host([mobile]) {
      flex-direction: row;
      width: 100%;
      height: 48px;
      padding: 4px 8px;
      padding-bottom: calc(4px + env(safe-area-inset-bottom));
      align-items: center;
      justify-content: space-between;
      overflow: visible;
      border-top: 1px solid #444;
      touch-action: manipulation;
    }

    :host([mobile][child-mode]) {
      height: auto;
      padding: 8px 8px;
      padding-bottom: calc(8px + env(safe-area-inset-bottom));
      justify-content: center;
    }

    :host([mobile]) .group {
      flex-direction: row;
    }

    :host([mobile]) .separator {
      width: 1px;
      height: 24px;
      margin: 0 4px;
    }

    :host([mobile]) .action-group {
      flex-direction: row;
      margin-top: 0;
      margin-left: auto;
    }

    .popover {
      display: none;
    }

    :host([mobile]) .popover {
      display: flex;
      flex-direction: column;
      position: absolute;
      bottom: calc(52px + env(safe-area-inset-bottom));
      left: 8px;
      right: 8px;
      max-height: calc(100dvh - 72px - env(safe-area-inset-bottom));
      box-sizing: border-box;
      overflow-y: auto;
      overscroll-behavior: contain;
      background: #2c2c2c;
      border: 1px solid #555;
      border-radius: 12px;
      padding: 8px;
      gap: 8px;
      z-index: 100;
      touch-action: manipulation;
      box-shadow: 0 -4px 16px rgba(0,0,0,0.4);
    }

    .popover > * {
      flex-shrink: 0;
    }

    .popover-backdrop {
      display: none;
    }

    :host([mobile]) .popover-backdrop {
      display: block;
      position: fixed;
      inset: 0;
      z-index: 99;
    }

    .popover .sub-tools {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      padding-bottom: 8px;
      border-bottom: 1px solid #444;
    }

    .popover-divider {
      height: 1px;
      background: #444;
    }

    .popover-label {
      font-size: 0.7rem;
      color: #888;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      padding: 0 4px;
    }

    .popover .project-list {
      display: flex;
      flex-direction: column;
      gap: 2px;
      max-height: 40vh;
      overflow-y: auto;
    }

    .popover .project-item {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 6px;
      width: 100%;
      padding: 8px 10px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      font-size: 0.85rem;
      text-align: left;
      cursor: pointer;
    }

    .popover .project-item:hover {
      background: #444;
    }

    .popover .project-item.current {
      background: #3a3a3a;
      color: #fff;
    }

    .popover .project-item .check {
      width: 16px;
      flex-shrink: 0;
      color: #5b8cf7;
    }

    .popover button.menu-btn {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 8px;
      width: 100%;
      height: auto;
      padding: 10px;
      font-size: 0.85rem;
      text-align: left;
      border-radius: 6px;
    }

    .popover button.menu-btn svg {
      flex-shrink: 0;
    }

    /* ── Child Mode ───────────────────────────── */
    .child-bar {
      display: flex;
      flex-direction: column;
      width: 100%;
      gap: 6px;
    }

    .child-colors {
      display: flex;
      justify-content: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .child-color-btn {
      width: 36px;
      height: 36px;
      border-radius: 50%;
      border: 3px solid transparent;
      padding: 0;
      cursor: pointer;
      transition: transform 0.15s ease, border-color 0.15s ease;
    }

    .child-color-btn:hover {
      transform: scale(1.15);
    }

    .child-color-btn.active {
      border-color: #fff;
      box-shadow: 0 0 0 2px #5b8cf7;
      transform: scale(1.15);
    }

    .child-tools-row {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 4px;
      flex-wrap: wrap;
    }

    .child-tool-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 48px;
      height: 48px;
      border: none;
      border-radius: 14px;
      background: #3a3a3a;
      color: #ccc;
      cursor: pointer;
      padding: 0;
      transition: all 0.15s ease;
    }

    .child-tool-btn svg {
      width: 22px;
      height: 22px;
    }

    .child-tool-btn:hover {
      background: #555;
      color: #fff;
    }

    .child-tool-btn.active {
      background: #5b8cf7;
      color: #fff;
    }

    .child-tool-btn:disabled {
      opacity: 0.3;
      cursor: default;
    }

    .child-size-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 40px;
      height: 48px;
      border: none;
      border-radius: 14px;
      background: #3a3a3a;
      color: #ccc;
      font-size: 0.85rem;
      font-weight: 700;
      cursor: pointer;
      padding: 0;
      transition: all 0.15s ease;
    }

    .child-size-btn:hover {
      background: #555;
      color: #fff;
    }

    .child-size-btn.active {
      background: #ff9500;
      color: #fff;
    }

    .child-sep {
      width: 1px;
      height: 32px;
      background: #555;
      margin: 0 2px;
      flex-shrink: 0;
    }

    .child-color-picker {
      width: 36px;
      height: 36px;
      border: none;
      border-radius: 50%;
      padding: 0;
      cursor: pointer;
      background: none;
    }
  `;ur([T()],ns.prototype,"_popoverGroup",2);ur([T()],ns.prototype,"_isFullscreen",2);ns=ur([de("app-toolbar")],ns);function As(e,t=0){let s=null,i=null,r=0;const n=()=>{s=null,i=null,r=performance.now(),e()};return{schedule(){if(s!==null||i!==null)return;const a=t-(performance.now()-r);a<=0?s=requestAnimationFrame(n):i=setTimeout(()=>{i=null,s=requestAnimationFrame(n)},a)},cancel(){s!==null&&cancelAnimationFrame(s),i!==null&&clearTimeout(i),s=null,i=null}}}function kn(e){return Math.max(0,Math.min(1,e))}function Mn(e){return e.pointerType==="mouse"?1:e.pointerType==="pen"?Number.isFinite(e.pressure)?kn(e.pressure):.5:e.pointerType==="touch"?1:e.pressure>0&&Number.isFinite(e.pressure)?kn(e.pressure):.5}function Cn(e){if(e.pointerType!=="pen")return{tiltX:0,tiltY:0};const{tiltX:t,tiltY:s,altitudeAngle:i,azimuthAngle:r}=e;if(Number.isFinite(t)&&Number.isFinite(s))return{tiltX:t,tiltY:s};if(!Number.isFinite(i)||!Number.isFinite(r))return{tiltX:0,tiltY:0};const n=Math.tan(i),a=o=>Math.atan2(Math.abs(o)<1e-9?0:o,n)*180/Math.PI;return{tiltX:a(Math.cos(r)),tiltY:a(Math.sin(r))}}function Sn(e){return e.pointerType!=="pen"||!Number.isFinite(e.twist)?0:(e.twist%360+360)%360}function ad(e,t,s,i,r=32,n=!1){const{width:a,height:o}=e.canvas,h=Math.floor(t),c=Math.floor(s);if(h<0||h>=a||c<0||c>=o)return null;const l=e.getImageData(0,0,a,o),d=l.data,u=cd(i),_=(c*a+h)*4,f=d[_],m=d[_+1],p=d[_+2],g=d[_+3];if(r===0&&f===u.r&&m===u.g&&p===u.b&&g===u.a)return null;const y=hd(a*o),w=Cs,x=k=>od(d,k*4,f,m,p,g,r),b=[h,c];let S=a,E=o,D=-1,L=-1;for(;b.length>0;){const k=b.pop(),M=b.pop(),P=k*a;if(y[P+M]===w||!x(P+M))continue;let R=M;for(;R>0&&y[P+R-1]!==w&&x(P+R-1);)R--;let O=M;for(;O<a-1&&y[P+O+1]!==w&&x(P+O+1);)O++;for(let V=R;V<=O;V++){const H=(P+V)*4;d[H]=u.r,d[H+1]=u.g,d[H+2]=u.b,n||(d[H+3]=u.a),y[P+V]=w}R<S&&(S=R),O>D&&(D=O),k<E&&(E=k),k>L&&(L=k),k>0&&Pn(b,y,w,x,a,k-1,R,O),k<o-1&&Pn(b,y,w,x,a,k+1,R,O)}if(D<0)return null;const I={x:S,y:E,w:D-S+1,h:L-E+1};return e.putImageData(l,0,0,I.x,I.y,I.w,I.h),I}function Pn(e,t,s,i,r,n,a,o){const h=n*r;let c=!1;for(let l=a;l<=o;l++){const d=t[h+l]!==s&&i(h+l);d&&!c&&e.push(l,n),c=d}}function od(e,t,s,i,r,n,a){return Math.abs(e[t]-s)<=a&&Math.abs(e[t+1]-i)<=a&&Math.abs(e[t+2]-r)<=a&&Math.abs(e[t+3]-n)<=a}let gs=new Uint16Array(0),Cs=0;function hd(e){return gs.length<e&&(gs=new Uint16Array(e),Cs=0),++Cs>65535&&(gs.fill(0),Cs=1),gs}const ys=new Map;let _e=null;function cd(e){const t=ys.get(e);if(t)return t;if(!_e){const o=document.createElement("canvas");o.width=1,o.height=1,_e=o.getContext("2d",{willReadFrequently:!0})}_e.clearRect(0,0,1,1),_e.fillStyle="#000",_e.fillStyle=e,_e.fillRect(0,0,1,1);const[s,i,r,n]=_e.getImageData(0,0,1,1).data,a={r:s,g:i,b:r,a:n};return ys.size>64&&ys.clear(),ys.set(e,a),a}function ld(e,t,s,i,r,n){e.save(),e.lineWidth=1,e.setLineDash([6,6]),e.strokeStyle="#ffffff",e.lineDashOffset=0,e.strokeRect(t+.5,s+.5,i,r),e.strokeStyle="#3b82f6",e.lineDashOffset=n,e.strokeRect(t+.5,s+.5,i,r),e.restore()}const Xa=8;function dd(e,t,s,i,r){const n=Xa/r,a=n/2;e.fillStyle="rgba(0, 0, 0, 0.5)",e.fillRect(0,0,s,t.y),e.fillRect(0,t.y+t.h,s,i-t.y-t.h),e.fillRect(0,t.y,t.x,t.h),e.fillRect(t.x+t.w,t.y,s-t.x-t.w,t.h),e.strokeStyle="#ffffff",e.lineWidth=1/r,e.setLineDash([]),e.strokeRect(t.x,t.y,t.w,t.h);const o=[{cx:t.x,cy:t.y},{cx:t.x+t.w/2,cy:t.y},{cx:t.x+t.w,cy:t.y},{cx:t.x+t.w,cy:t.y+t.h/2},{cx:t.x+t.w,cy:t.y+t.h},{cx:t.x+t.w/2,cy:t.y+t.h},{cx:t.x,cy:t.y+t.h},{cx:t.x,cy:t.y+t.h/2}];e.fillStyle="#ffffff",e.strokeStyle="#3b82f6",e.lineWidth=1/r;for(const{cx:f,cy:m}of o)e.fillRect(f-a,m-a,n,n),e.strokeRect(f-a,m-a,n,n);const h=`${Math.round(t.w)} × ${Math.round(t.h)}`,c=Math.max(10,12/r);e.font=`${c}px system-ui, sans-serif`,e.textAlign="right",e.textBaseline="top";const l=t.x+t.w,d=t.y+t.h+4/r,u=e.measureText(h),_=3/r;e.fillStyle="rgba(0, 0, 0, 0.7)",e.fillRect(l-u.width-_*2,d,u.width+_*2,c+_*2),e.fillStyle="#ffffff",e.fillText(h,l-_,d+_)}function Tn(e,t,s,i=Xa/2){const r=t.x>=e.x&&t.x<=e.x+e.w&&t.y>=e.y&&t.y<=e.y+e.h;r&&(i=Math.min(i,Math.min(Math.abs(e.w),Math.abs(e.h))*s/4));const n=i/s,a=[{handle:"nw",cx:e.x,cy:e.y},{handle:"n",cx:e.x+e.w/2,cy:e.y},{handle:"ne",cx:e.x+e.w,cy:e.y},{handle:"e",cx:e.x+e.w,cy:e.y+e.h/2},{handle:"se",cx:e.x+e.w,cy:e.y+e.h},{handle:"s",cx:e.x+e.w/2,cy:e.y+e.h},{handle:"sw",cx:e.x,cy:e.y+e.h},{handle:"w",cx:e.x,cy:e.y+e.h/2}];let o=null,h=1/0;for(const{handle:c,cx:l,cy:d}of a){const u=Math.max(Math.abs(t.x-l),Math.abs(t.y-d));u<=n&&u<h&&(h=u,o=c)}return o||(r?"move":null)}function _i(e){if(e==="free")return null;const t=e.split(":");if(t.length!==2)return null;const s=parseFloat(t[0]),i=parseFloat(t[1]);return!s||!i?null:s/i}function $n(e,t,s){const{x:i,y:r,w:n,h:a}=e,o=Math.abs(n),h=Math.abs(a),c=n>=0?1:-1,l=a>=0?1:-1;let d,u;s==="n"||s==="s"?(u=h,d=h*t):s==="e"||s==="w"||o/t>=h?(d=o,u=o/t):(u=h,d=h*t);const _=d*c,f=u*l,m=s==="nw"||s==="w"||s==="sw",p=s==="nw"||s==="n"||s==="ne",g=m?i+n-_:i,y=p?r+a-f:r;return{x:g,y,w:_,h:f}}const Ls=1.2;function js(e,t,s,i){const r=t.includes(" ")?`'${t}'`:t;return`${i?"italic ":""}${s?"bold ":""}${e}px ${r}`}function In(e,t,s,i,r,n,a,o,h){if(!t)return;e.save(),e.font=js(r,n,a,o),e.fillStyle=h,e.textBaseline="top";const c=r*Ls,l=t.split(`
`);for(let d=0;d<l.length;d++)e.fillText(l[d],s,i+d*c);e.restore()}function En(e,t,s,i,r,n){e.save(),e.font=js(s,i,r,n),e.textBaseline="top";const a=t.split(`
`),o=s*Ls,h=a.map(d=>e.measureText(d).width);let c=0;for(const d of h)d>c&&(c=d);const l=Math.max(1,a.length)*o;return e.restore(),{width:c,height:l,lineWidths:h}}const Rn={size:8,hitRadius:6,shape:"square",rotationStemLength:30,outsideDragThreshold:3},zs={size:20,hitRadius:20,shape:"circle",rotationStemLength:50,outsideDragThreshold:10},ud=4;function Gs(e){const t=e.width/2,s=e.height/2,i=e.skewX*Math.PI/180,r=e.skewY*Math.PI/180,n=Math.cos(e.rotation),a=Math.sin(e.rotation),o=(f,m)=>({a:f.a*m.a+f.c*m.b,b:f.b*m.a+f.d*m.b,c:f.a*m.c+f.c*m.d,d:f.b*m.c+f.d*m.d,e:f.a*m.e+f.c*m.f+f.e,f:f.b*m.e+f.d*m.f+f.f}),h=(f,m)=>({a:1,b:0,c:0,d:1,e:f,f:m}),c=()=>({a:n,b:a,c:-a,d:n,e:0,f:0}),l=()=>({a:1,b:0,c:Math.tan(i),d:1,e:0,f:0}),d=()=>({a:1,b:Math.tan(r),c:0,d:1,e:0,f:0}),u=()=>({a:e.scaleX,b:0,c:0,d:e.scaleY,e:0,f:0}),_=[h(e.x+t,e.y+s),c(),l(),d(),u(),h(-t,-s)].reduce(o,{a:1,b:0,c:0,d:1,e:0,f:0});return new DOMMatrix([_.a,_.b,_.c,_.d,_.e,_.f])}function vs(e,t){const s=Gs(t),i=s.a*s.d-s.b*s.c;return Math.abs(i)<1e-10?{x:e.x,y:e.y}:{x:(s.d*e.x-s.c*e.y+s.c*s.f-s.d*s.e)/i,y:(-s.b*e.x+s.a*e.y+s.b*s.e-s.a*s.f)/i}}function Wa(e,t){const s=Gs(t);return{x:s.a*e.x+s.c*e.y+s.e,y:s.b*e.x+s.d*e.y+s.f}}function mi(e){return Wa({x:e.width/2,y:e.height/2},e)}function pd(e,t){return Math.round(e/t)*t}function Dn(e){const{data:t,width:s,height:i}=e;let r=s,n=-1,a=-1,o=-1;for(let h=0;h<i;h++){const c=h*s*4+3;let l=0;for(;l<s&&t[c+l*4]===0;)l++;if(l===s)continue;n<0&&(n=h),o=h,l<r&&(r=l);let d=s-1;for(;d>a&&t[c+d*4]===0;)d--;d>a&&(a=d)}return n<0?null:{x:r,y:n,w:a-r+1,h:o-n+1}}function An(e,t){const s=Gs(e),i=(a,o)=>({x:s.a*a+s.c*o+s.e,y:s.b*a+s.d*o+s.f}),{width:r,height:n}=e;return[i(t.nw.x,t.nw.y),i(r+t.ne.x,t.ne.y),i(r+t.se.x,n+t.se.y),i(t.sw.x,n+t.sw.y)]}const fd=1e30;function Fi(e){const[t,s,i,r]=e,n=e.map(k=>({x:k.x-t.x,y:k.y-t.y})),a=s.x-t.x,o=s.y-t.y,h=r.x-t.x,c=r.y-t.y,l=t.x-s.x+i.x-r.x,d=t.y-s.y+i.y-r.y,u=l*c-d*h,_=a*c-o*h,f=yd(e);let m=0;for(let k=0;k<4;k++){const M=n[k],P=n[(k+1)%4];m+=M.x*P.y-P.x*M.y}const p=new Float64Array(16);for(let k=0;k<4;k++){const M=n[k],P=n[(k+1)%4],R=Math.hypot(P.x-M.x,P.y-M.y);if(R===0||m===0){p.set([0,0,fd,0],k*4);continue}const O=m>0?1:-1,V=-O*(P.y-M.y)/R,H=O*(P.x-M.x)/R;p.set([V,H,-(V*M.x+H*M.y),(Math.abs(V)+Math.abs(H))/2],k*4)}const g=new Float64Array(32),y=(k,M)=>{for(let P=0;P<M.length;P++){const R=M[P],O=M[(P+1)%M.length];g.set([R.x,R.y,O.x,O.y],k+P*4)}},[w,x,b,S]=n,E=Ln(w,x,b,S),D=E?null:Ln(x,b,S,w);E?(y(0,[E,x,b]),y(16,[E,S,w])):D?(y(0,[D,b,S]),y(16,[D,w,x])):y(0,n);const L=[0,16].map(k=>{let M=0,P=0,R=0;for(let O=k;O<k+16;O+=4)g[O]===g[O+2]&&g[O+1]===g[O+3]||(M+=g[O],P+=g[O+1],R++);return R>0?{x:M/R,y:P/R}:null}),I=L[0]??{x:0,y:0};return{ax:t.x,ay:t.y,quad:n,ex:a,ey:o,fx:h,fy:c,gx:l,gy:d,k2:u,ef:_,linear:Math.abs(u)<=1e-9*Math.abs(_)||Math.fround(u)===0,convex:f,empty:f&&m===0,lines:p,lobes:g,centroids:[I,L[1]??I]}}function Ln(e,t,s,i){const r=t.x-e.x,n=t.y-e.y,a=i.x-s.x,o=i.y-s.y,h=r*o-n*a;if(h===0)return null;const c=s.x-e.x,l=s.y-e.y,d=(c*o-l*a)/h,u=(c*n-l*r)/h;return d>0&&d<1&&u>0&&u<1?{x:e.x+d*r,y:e.y+d*n}:null}function _d(e,t,s){const{x:i,y:r,w:n,h:a}=s,o=new ImageData(n,a),h=Fi(t);if(h.empty)return o;const c=o.data,l=e.data,d=e.width,u=e.height,{quad:_,lines:f,lobes:m,convex:p}=h,g={x:0,y:0,ex:0,ey:0},y={u:0,v:0};for(let w=0;w<a;w++){const[x,b]=vd(t,r+w,i,n),S=r+w+.5-h.ay;for(let E=x;E<b;E++){const D=i+E+.5-h.ax;let L,I,k=1/0;if(p){let tt=!0;I=!0;let lt=0;for(;lt<16;lt+=4){const Yt=f[lt]*D+f[lt+1]*S+f[lt+2];if(Yt<=-f[lt+3])break;Yt<f[lt+3]&&(tt=!1),Yt<0&&(I=!1)}if(lt<16)continue;L=tt?1:jn(m,D-.5,S-.5)}else{const tt=On(_,D,S,g);I=yi(_,D,S)!==0,L=tt<.5?jn(m,D-.5,S-.5):I?1:0,k=Math.sqrt(tt)}if(L<=0)continue;let M=D,P=S;if(!I||k<.25){p&&On(_,D,S,g);const tt=Math.hypot(g.ex,g.ey);if(tt>0){const lt=-.25*g.ey/tt,Yt=.25*g.ex/tt;yi(_,g.x+lt,g.y+Yt)!==0?(M=g.x+lt,P=g.y+Yt):yi(_,g.x-lt,g.y-Yt)!==0?(M=g.x-lt,P=g.y-Yt):I||(M=g.x,P=g.y)}}if(!Ya(h,M,P,y)&&!md(h,M,P,y))continue;const R=y.u,O=y.v;let V=R*d-.5,H=O*u-.5;V<0?V=0:V>d-1&&(V=d-1),H<0?H=0:H>u-1&&(H=u-1);const F=V|0,nt=H|0,vt=F+1<d?F+1:F,G=nt+1<u?nt+1:nt,Ht=V-F,Xt=H-nt,Wt=(nt*d+F)*4,Mt=(nt*d+vt)*4,pt=(G*d+F)*4,st=(G*d+vt)*4,ct=l[Wt+3]*(1-Ht)*(1-Xt),N=l[Mt+3]*Ht*(1-Xt),K=l[pt+3]*(1-Ht)*Xt,Ct=l[st+3]*Ht*Xt,St=ct+N+K+Ct;if(St<=0)continue;const Pt=1/St,it=(w*n+E)*4;c[it]=(l[Wt]*ct+l[Mt]*N+l[pt]*K+l[st]*Ct)*Pt,c[it+1]=(l[Wt+1]*ct+l[Mt+1]*N+l[pt+1]*K+l[st+1]*Ct)*Pt,c[it+2]=(l[Wt+2]*ct+l[Mt+2]*N+l[pt+2]*K+l[st+2]*Ct)*Pt,c[it+3]=St*L}}return o}function Ya(e,t,s,i){const r=e.ef+t*e.gy-s*e.gx,n=t*e.ey-s*e.ex;let a,o;if(e.linear){if(r===0)return!1;o=-n/r,a=gi(e,t,s,o)}else{let h=r*r-4*n*e.k2;h<0&&(h=0);const c=Math.sqrt(h),l=-.5*(r>=0?r+c:r-c);if(o=r>=0?l/e.k2:n/l,a=gi(e,t,s,o),!(a>=0&&a<=1&&o>=0&&o<=1)&&l!==0){const d=r>=0?n/l:l/e.k2,u=gi(e,t,s,d);Bn(u,d)<Bn(a,o)&&(a=u,o=d)}}return a>-1&&a<2&&o>-1&&o<2?(i.u=a,i.v=o,!0):!1}function md(e,t,s,i){for(const r of e.centroids){const n=r.x-t,a=r.y-s,o=Math.hypot(n,a);if(o>0&&Ya(e,t+.01*n/o,s+.01*a/o,i))return!0}return!1}function gi(e,t,s,i){const r=e.ex+e.gx*i,n=e.ey+e.gy*i;return Math.abs(r)>Math.abs(n)?(t-e.fx*i)/r:n!==0?(s-e.fy*i)/n:1e30}function jn(e,t,s){let i=0,r=0;for(let a=0;a<16;a+=4)i+=zn(e[a],e[a+1],e[a+2],e[a+3],t,s);for(let a=16;a<32;a+=4)r+=zn(e[a],e[a+1],e[a+2],e[a+3],t,s);const n=Math.abs(i)+Math.abs(r);return n<1?n:1}function zn(e,t,s,i,r,n){if(t===i)return 0;const a=t<i?t:i,o=t<i?i:t,h=a>n?a:n,c=o<n+1?o:n+1;if(h>=c)return 0;const l=(s-e)/(i-t),d=e+(h-t)*l-r,u=e+(c-t)*l-r,_=(c-h)*gd(d,u);return i>t?_:-_}function gd(e,t){const s=e<t?e:t,i=e<t?t:e;if(i<=0)return 0;if(s>=1)return 1;if(i===s)return s;const r=s>0?s:0,n=i<1?i:1;return((i>1?i-1:0)+(n-r)*(r+n)/2)/(i-s)}function On(e,t,s,i){let r=1/0;for(let n=0;n<4;n++){const a=e[n],o=e[(n+1)%4],h=o.x-a.x,c=o.y-a.y,l=t-a.x,d=s-a.y,u=h*h+c*c;if(u===0)continue;let _=(l*h+d*c)/u;_<0?_=0:_>1&&(_=1);const f=l-h*_,m=d-c*_,p=f*f+m*m;p<r&&(r=p,i.x=a.x+h*_,i.y=a.y+c*_,i.ex=h,i.ey=c)}return r}function Bn(e,t){if(e!==e||t!==t)return 1/0;const s=e<0?-e:e>1?e-1:0,i=t<0?-t:t>1?t-1:0;return s>i?s:i}function yd(e){let t=!1,s=!1;for(let i=0;i<4;i++){const r=e[i],n=e[(i+1)%4],a=e[(i+2)%4],o=(n.x-r.x)*(a.y-n.y)-(n.y-r.y)*(a.x-n.x);o>0?t=!0:o<0&&(s=!0)}return!(t&&s)}function yi(e,t,s){let i=0;for(let r=0;r<4;r++){const n=e[r],a=e[(r+1)%4],o=(a.x-n.x)*(s-n.y)-(t-n.x)*(a.y-n.y);n.y<=s?a.y>s&&o>0&&i++:a.y<=s&&o<0&&i--}return i}function vd(e,t,s,i){const r=t-1,n=t+2;let a=1/0,o=-1/0;for(let h=0;h<4;h++){const c=e[h],l=e[(h+1)%4],d=Math.max(r,Math.min(c.y,l.y)),u=Math.min(n,Math.max(c.y,l.y));if(d>u)continue;let _=c.x,f=l.x;l.y!==c.y&&(_=c.x+(l.x-c.x)*(d-c.y)/(l.y-c.y),f=c.x+(l.x-c.x)*(u-c.y)/(l.y-c.y)),a=Math.min(a,_,f),o=Math.max(o,_,f)}return a>o?[0,0]:[Math.max(0,Math.floor(a-1.5)-s),Math.min(i,Math.ceil(o+1.5)-s)]}const bd=`#version 300 es
void main() {
  // One triangle covering the viewport.
  gl_Position = vec4(gl_VertexID == 1 ? 3.0 : -1.0, gl_VertexID == 2 ? 3.0 : -1.0, 0.0, 1.0);
}`,wd=`#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;

uniform sampler2D uSrc;
// The tile's top-left corner, and its height (gl_FragCoord counts from the bottom).
uniform vec2 uOrigin;
uniform float uTileH;
uniform vec2 uE, uF, uG;
uniform float uK2, uEF;
uniform bool uLinear, uConvex;
uniform vec2 uQuad[4];
uniform vec4 uLines[4];
uniform vec4 uLobes[8];
uniform vec2 uCentroids[2];
// Which of u and v the quad's corner order reverses (see QUAD_ORDERS).
uniform vec2 uFlip;
out vec4 outColor;

float meanClamped(float la, float lb) {
  float lo = min(la, lb), hi = max(la, lb);
  if (hi <= 0.0) return 0.0;
  if (lo >= 1.0) return 1.0;
  if (hi == lo) return lo;
  float a = max(lo, 0.0), b = min(hi, 1.0);
  return (max(hi - 1.0, 0.0) + (b - a) * (a + b) * 0.5) / (hi - lo);
}

float edgeArea(vec4 e, vec2 p0) {
  if (e.y == e.w) return 0.0;
  float ya = max(min(e.y, e.w), p0.y), yb = min(max(e.y, e.w), p0.y + 1.0);
  if (ya >= yb) return 0.0;
  float slope = (e.z - e.x) / (e.w - e.y);
  float la = e.x + (ya - e.y) * slope - p0.x, lb = e.x + (yb - e.y) * slope - p0.x;
  float area = (yb - ya) * meanClamped(la, lb);
  return e.w > e.y ? area : -area;
}

float pixelCoverage(vec2 p0) {
  float first = 0.0, second = 0.0;
  for (int i = 0; i < 4; i++) first += edgeArea(uLobes[i], p0);
  for (int i = 4; i < 8; i++) second += edgeArea(uLobes[i], p0);
  return min(abs(first) + abs(second), 1.0);
}

int winding(vec2 p) {
  int w = 0;
  for (int i = 0; i < 4; i++) {
    vec2 a = uQuad[i], b = uQuad[(i + 1) % 4];
    float side = (b.x - a.x) * (p.y - a.y) - (p.x - a.x) * (b.y - a.y);
    if (a.y <= p.y) {
      if (b.y > p.y && side > 0.0) w++;
    } else if (b.y <= p.y && side < 0.0) {
      w--;
    }
  }
  return w;
}

float nearestOnQuad(vec2 p, out vec2 nearest, out vec2 edge) {
  float best = 3.0e38;
  nearest = uQuad[0];
  edge = uQuad[1] - uQuad[0];
  for (int i = 0; i < 4; i++) {
    vec2 a = uQuad[i], q = uQuad[(i + 1) % 4] - a, w = p - a;
    float len2 = dot(q, q);
    if (len2 == 0.0) continue;
    float t = clamp(dot(w, q) / len2, 0.0, 1.0);
    vec2 d = w - q * t;
    float d2 = dot(d, d);
    if (d2 < best) {
      best = d2;
      nearest = a + q * t;
      edge = q;
    }
  }
  return best;
}

float outsideBy(float u, float v) {
  float du = u < 0.0 ? -u : u > 1.0 ? u - 1.0 : 0.0;
  float dv = v < 0.0 ? -v : v > 1.0 ? v - 1.0 : 0.0;
  return max(du, dv);
}

// Division by zero is undefined in GLSL, so where the CPU would get an
// infinity this takes a value far outside the source instead.
const float FAR = 1.0e30;

float uAt(vec2 h, float v) {
  float dx = uE.x + uG.x * v, dy = uE.y + uG.y * v;
  if (abs(dx) > abs(dy)) return (h.x - uF.x * v) / dx;
  return dy != 0.0 ? (h.y - uF.y * v) / dy : FAR;
}

bool unmapBilinear(vec2 h, out vec2 uv) {
  uv = vec2(0.0);
  float k1 = uEF + h.x * uG.y - h.y * uG.x;
  float k0 = h.x * uE.y - h.y * uE.x;
  float u, v;
  if (uLinear) {
    if (k1 == 0.0) return false;
    v = -k0 / k1;
    u = uAt(h, v);
  } else {
    float w = sqrt(max(k1 * k1 - 4.0 * k0 * uK2, 0.0));
    float q = -0.5 * (k1 >= 0.0 ? k1 + w : k1 - w);
    // With q == 0 both roots are 0.
    v = k1 >= 0.0 ? q / uK2 : q != 0.0 ? k0 / q : 0.0;
    u = uAt(h, v);
    if (!(u >= 0.0 && u <= 1.0 && v >= 0.0 && v <= 1.0) && q != 0.0) {
      float v2 = k1 >= 0.0 ? k0 / q : q / uK2;
      float u2 = uAt(h, v2);
      if (outsideBy(u2, v2) < outsideBy(u, v)) {
        u = u2;
        v = v2;
      }
    }
  }
  uv = vec2(u, v);
  return u > -1.0 && u < 2.0 && v > -1.0 && v < 2.0;
}

void main() {
  outColor = vec4(0.0);
  vec2 c = uOrigin + vec2(gl_FragCoord.x, uTileH - gl_FragCoord.y);
  float coverage;
  bool inside = true;
  float dist = 3.0e38;
  vec2 nearest = vec2(0.0), edge = vec2(0.0);
  if (uConvex) {
    bool full = true;
    for (int i = 0; i < 4; i++) {
      float d = dot(uLines[i].xy, c) + uLines[i].z;
      if (d <= -uLines[i].w) return;
      if (d < uLines[i].w) full = false;
      if (d < 0.0) inside = false;
    }
    coverage = full ? 1.0 : pixelCoverage(c - 0.5);
  } else {
    float d2 = nearestOnQuad(c, nearest, edge);
    inside = winding(c) != 0;
    coverage = d2 < 0.5 ? pixelCoverage(c - 0.5) : inside ? 1.0 : 0.0;
    dist = sqrt(d2);
  }
  if (coverage <= 0.0) return;

  vec2 h = c;
  if (!inside || dist < 0.25) {
    if (uConvex) nearestOnQuad(c, nearest, edge);
    float len = length(edge);
    if (len > 0.0) {
      vec2 n = vec2(-edge.y, edge.x) * (0.25 / len);
      if (winding(nearest + n) != 0) h = nearest + n;
      else if (winding(nearest - n) != 0) h = nearest - n;
      else if (!inside) h = nearest;
    }
  }
  vec2 uv;
  if (!unmapBilinear(h, uv)) {
    bool mapped = false;
    for (int i = 0; i < 2 && !mapped; i++) {
      vec2 d = uCentroids[i] - h;
      float len = length(d);
      if (len > 0.0) mapped = unmapBilinear(h + d * (0.01 / len), uv);
    }
    if (!mapped) return;
  }

  uv = mix(uv, 1.0 - uv, uFlip);
  ivec2 size = textureSize(uSrc, 0);
  vec2 st = clamp(uv * vec2(size) - 0.5, vec2(0.0), vec2(size - 1));
  ivec2 i0 = ivec2(st);
  ivec2 i1 = min(i0 + 1, size - 1);
  vec2 t = st - vec2(i0);
  vec4 c00 = texelFetch(uSrc, i0, 0);
  vec4 c10 = texelFetch(uSrc, ivec2(i1.x, i0.y), 0);
  vec4 c01 = texelFetch(uSrc, ivec2(i0.x, i1.y), 0);
  vec4 c11 = texelFetch(uSrc, i1, 0);
  // Weighted by alpha, so the result is premultiplied, as the canvas wants it.
  float a00 = c00.a * (1.0 - t.x) * (1.0 - t.y), a10 = c10.a * t.x * (1.0 - t.y);
  float a01 = c01.a * (1.0 - t.x) * t.y, a11 = c11.a * t.x * t.y;
  outColor = vec4(c00.rgb * a00 + c10.rgb * a10 + c01.rgb * a01 + c11.rgb * a11, a00 + a10 + a01 + a11) * coverage;
}`,xd=["uSrc","uOrigin","uTileH","uE","uF","uG","uK2","uEF","uLinear","uConvex","uQuad","uLines","uLobes","uCentroids","uFlip"],kd=[{order:[0,1,2,3],flip:[0,0]},{order:[1,0,3,2],flip:[1,0]},{order:[2,3,0,1],flip:[1,1]},{order:[3,2,1,0],flip:[0,1]}],Md=2048,Cd=8192*8192;let _t;const Ni=new WeakSet;function Sd(){if(typeof WebGL2RenderingContext>"u")return null;const e=document.createElement("canvas");e.width=e.height=1;const t=e.getContext("webgl2",{alpha:!0,premultipliedAlpha:!0,antialias:!1,depth:!1,stencil:!1,preserveDrawingBuffer:!0,failIfMajorPerformanceCaveat:!0});if(!t)return null;const s=t.createProgram(),i=[[t.VERTEX_SHADER,bd],[t.FRAGMENT_SHADER,wd]];for(const[h,c]of i){const l=t.createShader(h);t.shaderSource(l,c),t.compileShader(l),t.attachShader(s,l)}if(t.linkProgram(s),t.isContextLost())return;if(!t.getProgramParameter(s,t.LINK_STATUS)){const h=t.getAttachedShaders(s)?.map(c=>t.getShaderInfoLog(c)).filter(Boolean)??[];return console.warn("Perspective warp shader failed; warping on the CPU.",t.getProgramInfoLog(s),...h),null}t.disable(t.DITHER),t.useProgram(s);const r=Object.fromEntries(xd.map(h=>[h,t.getUniformLocation(s,h)]));t.uniform1i(r.uSrc,0);const n=t.createTexture();t.bindTexture(t.TEXTURE_2D,n),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_MIN_FILTER,t.NEAREST),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_MAG_FILTER,t.NEAREST),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_WRAP_S,t.CLAMP_TO_EDGE),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_WRAP_T,t.CLAMP_TO_EDGE),t.pixelStorei(t.UNPACK_PREMULTIPLY_ALPHA_WEBGL,!1),t.pixelStorei(t.UNPACK_FLIP_Y_WEBGL,!1),t.pixelStorei(t.UNPACK_COLORSPACE_CONVERSION_WEBGL,t.NONE);const a=t.getParameter(t.MAX_VIEWPORT_DIMS),o=Math.min(Md,a[0],a[1],t.getParameter(t.MAX_RENDERBUFFER_SIZE));return e.addEventListener("webglcontextlost",()=>{_t?.canvas===e&&(_t=void 0)}),{canvas:e,gl:t,uniforms:r,texture:n,maxTexture:t.getParameter(t.MAX_TEXTURE_SIZE),tile:o,source:null}}function Pd(){return _t===void 0&&(_t=Sd(),_t===void 0)?null:(_t?.gl.isContextLost()&&(_t=void 0),_t??null)}function Ua(e){const t=Pd();return!!t&&e.width<=t.maxTexture&&e.height<=t.maxTexture&&e.width*e.height<=Cd&&!Ni.has(e)}function Td(e,t,s,i){if(!Ua(e))return!1;const r=_t,{gl:n,uniforms:a}=r;if(r.source!==e){for(r.source=null;n.getError()!==n.NO_ERROR&&!n.isContextLost(););if(Ni.add(e),n.texImage2D(n.TEXTURE_2D,0,n.RGBA8,e.width,e.height,0,n.RGBA,n.UNSIGNED_BYTE,new Uint8Array(e.data.buffer,e.data.byteOffset,e.data.byteLength)),n.getError()!==n.NO_ERROR||n.isContextLost())return!1;Ni.delete(e),r.source=e}const o=Math.min(s.w,r.tile),h=Math.min(s.h,r.tile);if((r.canvas.width<o||r.canvas.height<h)&&(r.canvas.width=Math.max(r.canvas.width,o),r.canvas.height=Math.max(r.canvas.height,h)),n.drawingBufferWidth!==r.canvas.width||n.drawingBufferHeight!==r.canvas.height)return n.isContextLost()?(_t=void 0,!1):(r.canvas.width=r.canvas.height=1,n.deleteTexture(r.texture),n.getExtension("WEBGL_lose_context")?.loseContext(),_t=null,!1);const c=[Fi(t)],l=c[0];let d=-1;const u=m=>{if(m===d)return;d=m;const{order:p,flip:g}=kd[m],y=c[m]??=Fi(p.map(w=>t[w]));n.uniform2f(a.uE,y.ex,y.ey),n.uniform2f(a.uF,y.fx,y.fy),n.uniform2f(a.uG,y.gx,y.gy),n.uniform1f(a.uK2,y.k2),n.uniform1f(a.uEF,y.ef),n.uniform1i(a.uLinear,y.linear?1:0),n.uniform1i(a.uConvex,y.convex?1:0),n.uniform2fv(a.uQuad,y.quad.flatMap(w=>[w.x,w.y])),n.uniform4fv(a.uLines,Float32Array.from(y.lines)),n.uniform4fv(a.uLobes,Float32Array.from(y.lobes)),n.uniform2fv(a.uCentroids,y.centroids.flatMap(w=>[w.x,w.y])),n.uniform2f(a.uFlip,g[0],g[1])},_=t.map((m,p)=>Math.min(Math.hypot(t[(p+1)%4].x-m.x,t[(p+1)%4].y-m.y),Math.hypot(t[(p+3)%4].x-m.x,t[(p+3)%4].y-m.y))),f=(m,p,g,y)=>{let w=0,x=1/0;return t.forEach((b,S)=>{const E=Math.max(m-b.x,0,b.x-(m+g)),D=Math.max(p-b.y,0,b.y-(p+y)),L=E*E+D*D;(L<x||L===x&&_[S]<_[w])&&(x=L,w=S)}),w};i.save(),i.setTransform(1,0,0,1,0,0),i.globalAlpha=1,i.globalCompositeOperation="source-over",i.imageSmoothingEnabled=!1;for(let m=0;m<s.h;m+=r.tile)for(let p=0;p<s.w;p+=r.tile){const g=Math.min(r.tile,s.w-p),y=Math.min(r.tile,s.h-m);if(l.empty){i.clearRect(p,m,g,y);continue}const w=l.convex?f(s.x+p,s.y+m,g,y):0;u(w);const x=c[w];n.viewport(0,0,g,y),n.uniform2f(a.uOrigin,s.x+p-x.ax,s.y+m-x.ay),n.uniform1f(a.uTileH,y),n.drawArrays(n.TRIANGLES,0,3),i.clearRect(p,m,g,y),i.drawImage(r.canvas,0,r.canvas.height-y,g,y,p,m,g,y)}return i.restore(),!n.isContextLost()}function $d(e){if(!_t||_t.source!==e)return;const{gl:t}=_t;t.texImage2D(t.TEXTURE_2D,0,t.RGBA8,1,1,0,t.RGBA,t.UNSIGNED_BYTE,null),_t.source=null,_t.canvas.width=_t.canvas.height=1}const Id=4096*4096,Ed=16384,Va=new WeakMap;function Ga(e,t,s){let i=Number.isFinite(s)&&s>0?s:1;const r=Math.min(Math.sqrt(Id/Math.max(1,e*t)),Ed/Math.max(1,e,t));return i>r&&(i=Math.max(Math.min(i,1),r)),{width:Math.max(1,Math.round(e*i)),height:Math.max(1,Math.round(t*i))}}function Hn(e,t,s,i){const r=Ga(t,s,i);e.width=r.width,e.height=r.height,e.style.width=`${t}px`,e.style.height=`${s}px`,Va.set(e,{width:t,height:s,backingWidth:r.width,backingHeight:r.height}),e.getContext("2d")?.setTransform(r.width/t,0,0,r.height/s,0,0)}function Os(e){const t=Va.get(e);return t&&t.backingWidth===e.width&&t.backingHeight===e.height?{width:t.width,height:t.height}:{width:e.width,height:e.height}}const Ft=(e,t)=>({x:(e.x+t.x)/2,y:(e.y+t.y)/2});function qs(e){const[t,s,i,r]=e;return{nw:t,n:Ft(t,s),ne:s,e:Ft(s,i),se:i,s:Ft(i,r),sw:r,w:Ft(r,t)}}function pr(e){return{x:(e[0].x+e[1].x+e[2].x+e[3].x)/4,y:(e[0].y+e[1].y+e[2].y+e[3].y)/4}}function fr(e,t,s){const i=Ft(e[0],e[1]),r=pr(e);let n=i.x-r.x,a=i.y-r.y,o=Math.sqrt(n*n+a*a);if(o<1){n=e[1].y-e[0].y,a=e[0].x-e[1].x;const c=Ft(e[2],e[3]);if(n*(i.x-c.x)+a*(i.y-c.y)<0&&(n=-n,a=-a),o=Math.sqrt(n*n+a*a),o<1)return i}const h=t.rotationStemLength/s;return{x:i.x+n/o*h,y:i.y+a/o*h}}function Xi(e,t,s,i){const r=qs(t),n=s.hitRadius/i,a=Bs(e,t),o=Math.hypot(r.e.x-r.w.x,r.e.y-r.w.y),h=Math.hypot(r.s.x-r.n.x,r.s.y-r.n.y);let c=null,l=1/0;for(const[d,u]of Object.entries(r)){let _=n;if(a){const g=d==="e"||d==="w"?o:d==="n"||d==="s"?h:Math.min(o,h);_=Math.min(n,g/4)}const f=e.x-u.x,m=e.y-u.y,p=f*f+m*m;p<=_*_&&p<=l&&(l=p,c=d)}return c}function Wi(e,t,s,i){const r=fr(t,s,i),n=s.hitRadius/i,a=e.x-r.x,o=e.y-r.y;return a*a+o*o<=n*n}function Bs(e,t){const{x:s,y:i}=e;let r=0;for(let n=0;n<4;n++){const a=t[n],o=t[(n+1)%4],h=(o.x-a.x)*(i-a.y)-(s-a.x)*(o.y-a.y);if(h===0&&Math.min(a.x,o.x)<=s&&s<=Math.max(a.x,o.x)&&Math.min(a.y,o.y)<=i&&i<=Math.max(a.y,o.y))return!0;a.y<=i?o.y>i&&h>0&&r++:o.y<=i&&h<0&&r--}return r!==0}function Rd(e,t,s,i){const r=qs(t),n=s.size/2/i;e.save(),e.fillStyle="#ffffff",e.strokeStyle="#3b82f6",e.lineWidth=1.5/i;for(const a of Object.values(r))s.shape==="circle"?(e.beginPath(),e.arc(a.x,a.y,n,0,Math.PI*2),e.fill(),e.stroke()):(e.fillRect(a.x-n,a.y-n,n*2,n*2),e.strokeRect(a.x-n,a.y-n,n*2,n*2));e.restore()}function Dd(e,t,s,i){const r=Ft(t[0],t[1]),n=fr(t,s,i),a=(s.shape==="circle"?8:6)/i;e.save(),e.strokeStyle="#3b82f6",e.lineWidth=1.5/i,e.fillStyle="#ffffff",e.beginPath(),e.moveTo(r.x,r.y),e.lineTo(n.x,n.y),e.stroke(),e.beginPath(),e.arc(n.x,n.y,a,0,Math.PI*2),e.fill(),e.stroke(),e.restore()}function Ad(e,t,s,i=1){const r=e[i],n=pr(e),a=x=>{const b=Math.hypot(x.x-n.x,x.y-n.y);return b>0?{x:(x.x-n.x)/b,y:(x.y-n.y)/b}:{x:0,y:0}},o=a(i<2?Ft(e[0],e[1]):Ft(e[2],e[3])),h=a(i===1||i===2?Ft(e[1],e[2]):Ft(e[3],e[0]));let c=o.x+h.x,l=o.y+h.y,d=Math.sqrt(c*c+l*l);d<1e-6&&(c=r.x-n.x,l=r.y-n.y,d=Math.sqrt(c*c+l*l));const u=t.shape==="circle",_=(u?38:30)/s,f=(u?22:12)/s,m=(u?48:28)/s,p=d>1e-6?c/d:Math.SQRT1_2,g=d>1e-6?l/d:-Math.SQRT1_2,y=h.x||h.y?h.x:1,w=h.x||h.y?h.y:0;return{commitCenter:{x:r.x+p*_,y:r.y+g*_},cancelCenter:{x:r.x+p*_+y*m,y:r.y+g*_+w*m},buttonRadius:f}}function Ld(e,t,s){const{commitCenter:i,cancelCenter:r,buttonRadius:n}=t;e.save(),e.lineCap="round",e.lineJoin="round",e.fillStyle="#ffffff",e.strokeStyle="#22c55e",e.lineWidth=1.5/s,e.beginPath(),e.arc(i.x,i.y,n,0,Math.PI*2),e.fill(),e.stroke(),e.strokeStyle="#16a34a",e.lineWidth=1.5/s,e.beginPath();const a=n*.4;e.moveTo(i.x-a,i.y+a*.1),e.lineTo(i.x-a*.15,i.y+a*.65),e.lineTo(i.x+a,i.y-a*.55),e.stroke(),e.fillStyle="#ffffff",e.strokeStyle="#ef4444",e.lineWidth=1.5/s,e.beginPath(),e.arc(r.x,r.y,n,0,Math.PI*2),e.fill(),e.stroke(),e.strokeStyle="#dc2626",e.lineWidth=1.5/s,e.beginPath();const o=n*.32;e.moveTo(r.x-o,r.y-o),e.lineTo(r.x+o,r.y+o),e.moveTo(r.x+o,r.y-o),e.lineTo(r.x-o,r.y+o),e.stroke(),e.restore()}const jd={nw:"nwse-resize",ne:"nesw-resize",se:"nwse-resize",sw:"nesw-resize",n:"ns-resize",s:"ns-resize",e:"ew-resize",w:"ew-resize"};function Ss(e,t){const s=qs(t),i=pr(t),r=l=>{const d=s[l],u=Math.hypot(d.x-i.x,d.y-i.y);return u>1e-9?{x:(d.x-i.x)/u,y:(d.y-i.y)/u}:{x:0,y:0}},[n,a]=e.length===2?[r(e[0]),r(e[1])]:[r(e),{x:0,y:0}],o=n.x+a.x,h=n.y+a.y;if(Math.hypot(o,h)<1e-9)return jd[e];const c=(Math.atan2(h,o)*180/Math.PI+360)%180;return c<22.5||c>=157.5?"ew-resize":c<67.5?"nwse-resize":c<112.5?"ns-resize":"nesw-resize"}function zd(e,t,s,i){if(Wi(e,t,s,i))return"grab";const r=Xi(e,t,s,i);return r?Ss(r,t):Bs(e,t)?"move":"crosshair"}const Od=4096*4096,Bd=1024*1024,Hd=2048*2048,Fd=8192*8192,vi=["nw","ne","se","sw"];let Nd=1;class ne{constructor(t,s,i,r,n){this._id=Nd++,this._perspectiveCorners={nw:{x:0,y:0},ne:{x:0,y:0},se:{x:0,y:0},sw:{x:0,y:0}},this._perspectiveActive=!1,this._warpCache=null,this._draftWarpCache=null,this._interaction={type:"idle"},this._handleConfig=Rn,this._sourceImageData=t,this._sourceRect=s,this._previewCanvas=i,this._zoom=r,this._pan=n,this._sourceCanvas=document.createElement("canvas"),this._sourceCanvas.width=t.width,this._sourceCanvas.height=t.height,this._sourceCanvas.getContext("2d").putImageData(t,0,0),this._state={x:s.x,y:s.y,width:s.w,height:s.h,rotation:0,skewX:0,skewY:0,scaleX:1,scaleY:1},this._initialState={...this._state},this.renderPreview()}get x(){return this._state.x}set x(t){this._state.x=t,this._onChange()}get y(){return this._state.y}set y(t){this._state.y=t,this._onChange()}get width(){return Math.abs(this._state.width*this._state.scaleX)}set width(t){if(t<=0)return;const s=this._state,i=t/this.width;s.x+=(s.width-s.width*i)/2,s.width*=i;for(const r of vi)this._perspectiveCorners[r].x*=i;this._onChange()}get height(){return Math.abs(this._state.height*this._state.scaleY)}set height(t){if(t<=0)return;const s=this._state,i=t/this.height;s.y+=(s.height-s.height*i)/2,s.height*=i;for(const r of vi)this._perspectiveCorners[r].y*=i;this._onChange()}get rotation(){return this._state.rotation*180/Math.PI}set rotation(t){this._state.rotation=t*Math.PI/180,this._onChange()}get skewX(){return this._state.skewX}set skewX(t){this._state.skewX=Math.max(-89,Math.min(89,t)),this._onChange()}get skewY(){return this._state.skewY}set skewY(t){this._state.skewY=Math.max(-89,Math.min(89,t)),this._onChange()}get flipH(){return this._state.scaleX<0}set flipH(t){const s=t,i=this._state.scaleX<0;s!==i&&(this._state.scaleX=-this._state.scaleX,this._onChange())}get flipV(){return this._state.scaleY<0}set flipV(t){const s=t,i=this._state.scaleY<0;s!==i&&(this._state.scaleY=-this._state.scaleY,this._onChange())}get perspectiveActive(){return this._perspectiveActive}get touchMode(){return this._handleConfig===zs}setTouchMode(t){this._handleConfig=t?zs:Rn,this.renderPreview()}onPointerDown(t,s,i){const r=this.buttonAt(t);if(r)return this.pressButton(r),!0;if(Wi(t,this._getCorners(),this._handleConfig,this._zoom)){const a=mi(this._state),o=Math.atan2(t.y-a.y,t.x-a.x);return this._interaction={type:"rotating",startAngle:o,startRotation:this._state.rotation},!0}const n=Xi(t,this._getCorners(),this._handleConfig,this._zoom);return n?(s.ctrl&&(n==="nw"||n==="ne"||n==="se"||n==="sw")?(this._perspectiveActive=!0,this._interaction={type:"perspective",corner:n,startPoint:t,startOffset:{...this._perspectiveCorners[n]}}):s.ctrl&&(n==="n"||n==="e"||n==="s"||n==="w")?this._interaction={type:"skewing",edge:n,startPoint:t,startSkewX:this._state.skewX,startSkewY:this._state.skewY}:this._interaction={type:"resizing",handle:n,origin:{point:t,state:{...this._state}}},!0):Bs(t,this._getCorners())?(this._interaction={type:"moving",startPoint:t,startX:this._state.x,startY:this._state.y},!0):(this._interaction={type:"outside-pending",startPoint:t,slop:i??this._handleConfig.outsideDragThreshold},!0)}onPointerMove(t,s){switch(this._interaction.type){case"moving":return this._handleMove(t,s),!0;case"resizing":return this._handleResize(t,s),!0;case"rotating":return this._handleRotate(t,s),!0;case"skewing":return this._handleSkew(t),!0;case"perspective":return this._handlePerspective(t),!0;case"outside-pending":{const i=t.x-this._interaction.startPoint.x,r=t.y-this._interaction.startPoint.y;if(Math.sqrt(i*i+r*r)*this._zoom>this._interaction.slop){const a=mi(this._state),o=Math.atan2(this._interaction.startPoint.y-a.y,this._interaction.startPoint.x-a.x);return this._interaction={type:"rotating",startAngle:o,startRotation:this._state.rotation},this._handleRotate(t,s),!0}return!1}}return!1}onPointerUp(t){const s=this._interaction;if(this._interaction={type:"idle"},s.type==="button")return Math.hypot(t.x-s.center.x,t.y-s.center.y)<=s.radius?`${s.button}-button`:null;if(s.type==="outside-pending"){const i=this.hitTestButton(t);return i?`${i}-button`:"commit"}return null}hitTestButton(t){return this.buttonAt(t)?.button??null}buttonAt(t){const{commitCenter:s,cancelCenter:i,buttonRadius:r}=this.getButtons();return Math.hypot(t.x-s.x,t.y-s.y)<=r?{button:"commit",center:s,radius:r}:Math.hypot(t.x-i.x,t.y-i.y)<=r?{button:"cancel",center:i,radius:r}:null}pressButton(t){this._interaction={type:"button",...t}}getButtons(){const t=this._placeButtons(),{commitCenter:s,cancelCenter:i}=t;return i.x+i.y<s.x+s.y?{...t,commitCenter:i,cancelCenter:s}:t}_placeButtons(){const t=this._getCorners(),s=this._handleConfig,i=this._zoom,r=[1,0,2,3],n=r.map(k=>Ad(t,s,i,k)),a=n[0].buttonRadius,o=-this._pan.x/i,h=-this._pan.y/i,c=Os(this._previewCanvas),l=(c.width-this._pan.x)/i,d=(c.height-this._pan.y)/i,u=a+(this.touchMode?12/i:0),_=o+u,f=h+u,m=Math.max(_,l-u),p=Math.max(f,d-u),g=(k,M=u)=>k.x>=o+M&&k.x<=l-M&&k.y>=h+M&&k.y<=d-M,y={x:(t[0].x+t[1].x+t[2].x+t[3].x)/4,y:(t[0].y+t[1].y+t[2].y+t[3].y)/4},w=[...Object.values(qs(t)),fr(t,s,i),y],x=a+s.size/2/i,b=k=>Math.min(...[k.commitCenter,k.cancelCenter].flatMap(M=>w.map(P=>Math.hypot(P.x-M.x,P.y-M.y)))),S=(k,M=u)=>g(k.commitCenter,M)&&g(k.cancelCenter,M)&&b(k)>=x,E=n.map((k,M)=>{const P=t[r[M]],R=O=>({x:2*P.x-O.x,y:2*P.y-O.y});return{commitCenter:R(k.commitCenter),cancelCenter:R(k.cancelCenter),buttonRadius:a}}),D=k=>{const M=(O,V,H,F)=>{const nt=Math.min(Math.max(O,H),F)-O,vt=Math.min(Math.max(V,H),F)-V;return Math.abs(nt)>Math.abs(vt)?nt:vt},P=M(k.commitCenter.x,k.cancelCenter.x,_,m),R=M(k.commitCenter.y,k.cancelCenter.y,f,p);return{commitCenter:{x:k.commitCenter.x+P,y:k.commitCenter.y+R},cancelCenter:{x:k.cancelCenter.x+P,y:k.cancelCenter.y+R},buttonRadius:a}},L=[...n,...E].map(D),I=n.find(k=>S(k))??n.find(k=>S(k,a))??[...E,...L].find(k=>S(k));return I||L.reduce((k,M)=>b(M)>b(k)?M:k)}cancelInteraction(t=!1){const s=this._interaction;if(s.type==="idle")return!1;if(this._interaction={type:"idle"},t){const i=this._state;switch(s.type){case"moving":i.x=s.startX,i.y=s.startY;break;case"resizing":s.origin.state&&Object.assign(i,s.origin.state);break;case"rotating":i.rotation=s.startRotation;break;case"skewing":i.skewX=s.startSkewX,i.skewY=s.startSkewY;break;case"perspective":this._perspectiveCorners[s.corner]={...s.startOffset};break}this._onChange()}return!0}_handleMove(t,s){const i=this._interaction;if(i.type!=="moving")return;let r=Math.round(t.x-i.startPoint.x),n=Math.round(t.y-i.startPoint.y);s.shift&&(Math.abs(r)>Math.abs(n)?n=0:r=0),this._state.x=i.startX+r,this._state.y=i.startY+n,this._onChange()}_handleResize(t,s){const i=this._interaction;if(i.type!=="resizing")return;const{handle:r,origin:n}=i,a=n.state,o=vs(n.point,a),h=vs(t,a),c=h.x-o.x,l=h.y-o.y;let d=a.width,u=a.height;if(r.includes("e")&&(d=a.width+c),r.includes("w")&&(d=a.width-c),r.includes("s")&&(u=a.height+l),r.includes("n")&&(u=a.height-l),s.shift&&(r==="nw"||r==="ne"||r==="se"||r==="sw")){const m=a.width/a.height;Math.abs(d/u)>m?u=(u<0?-1:1)*Math.abs(d)/m:d=(d<0?-1:1)*Math.abs(u)*m}const _=ud/this._zoom;Math.abs(d)<_&&(d=d<0?-_:_),Math.abs(u)<_&&(u=u<0?-_:_);const f=Wa({x:r.includes("w")?a.width-d/2:r.includes("e")?d/2:a.width/2,y:r.includes("n")?a.height-u/2:r.includes("s")?u/2:a.height/2},a);this._state.width=Math.abs(d),this._state.height=Math.abs(u),this._state.scaleX=d<0?-a.scaleX:a.scaleX,this._state.scaleY=u<0?-a.scaleY:a.scaleY,this._state.x=f.x-this._state.width/2,this._state.y=f.y-this._state.height/2,this._onChange()}_handleRotate(t,s){const i=this._interaction;if(i.type!=="rotating")return;const r=mi(this._state),n=Math.atan2(t.y-r.y,t.x-r.x);let a=i.startRotation+(n-i.startAngle);s.shift&&(a=pd(a,Math.PI/12)),this._state.rotation=a,this._onChange()}_handleSkew(t){const s=this._interaction;if(s.type!=="skewing")return;const i=t.x-s.startPoint.x,r=t.y-s.startPoint.y;if(s.edge==="n"||s.edge==="s"){const n=s.edge==="n"?-1:1;this._state.skewX=Math.max(-89,Math.min(89,s.startSkewX+n*i*.5))}else{const n=s.edge==="w"?-1:1;this._state.skewY=Math.max(-89,Math.min(89,s.startSkewY+n*r*.5))}this._onChange()}_handlePerspective(t){const s=this._interaction;if(s.type!=="perspective")return;const i=vs(s.startPoint,this._state),r=vs(t,this._state);this._perspectiveCorners[s.corner]={x:s.startOffset.x+r.x-i.x,y:s.startOffset.y+r.y-i.y},this._onChange()}renderPreview(){const t=this._previewCanvas.getContext("2d"),{width:s,height:i}=Os(this._previewCanvas);t.clearRect(0,0,s,i),t.save(),t.translate(this._pan.x,this._pan.y),t.scale(this._zoom,this._zoom);const r=this._getCorners();t.save(),t.lineWidth=1/this._zoom,t.setLineDash([6/this._zoom,6/this._zoom]),t.strokeStyle="#ffffff",t.lineDashOffset=0,t.beginPath(),t.moveTo(r[0].x,r[0].y);for(let n=1;n<4;n++)t.lineTo(r[n].x,r[n].y);t.closePath(),t.stroke(),t.strokeStyle="#3b82f6",t.lineDashOffset=4/this._zoom,t.beginPath(),t.moveTo(r[0].x,r[0].y);for(let n=1;n<4;n++)t.lineTo(r[n].x,r[n].y);t.closePath(),t.stroke(),t.restore(),Rd(t,r,this._handleConfig,this._zoom),Dd(t,r,this._handleConfig,this._zoom),Ld(t,this.getButtons(),this._zoom),t.restore()}renderTransformed(t,s){s?this._render(t,s,1/0):this._render(t,null,this._perspectiveActive?this._warpPixelBudget():1/0)}_render(t,s,i){if(this._perspectiveActive){const r=this._getWarp(s,i);if(!r)return;if(r.scale===1)t.drawImage(r.canvas,r.x,r.y);else{const n=this._getSnapshotBounds();t.save(),t.beginPath(),t.rect(n.x,n.y,n.w,n.h),t.clip(),t.drawImage(r.canvas,r.x,r.y,r.w,r.h),t.restore()}}else{const r=this._matrix();t.save(),t.transform(r.a,r.b,r.c,r.d,r.e,r.f),t.drawImage(this._sourceCanvas,0,0,this._state.width,this._state.height),t.restore()}}_warpPixelBudget(){const{type:t}=this._interaction;return t==="idle"||t==="button"||t==="outside-pending"?Od:Ua(this._sourceImageData)?Hd:Bd}_getWarp(t,s){const i=An(this._state,this._perspectiveCorners),r=this._getSnapshotBounds();let{x:n,y:a,w:o,h}=r;if(t){const b=Math.min(n+o,t.x+t.w),S=Math.min(a+h,t.y+t.h);n=Math.max(n,t.x),a=Math.max(a,t.y),o=b-n,h=S-a}if(o<=0||h<=0)return null;let c=Math.min(1,Math.sqrt(s/(o*h))),l,d,u,_;for(;l=Math.floor(n*c),d=Math.floor(a*c),u=Math.ceil((n+o)*c)-l,_=Math.ceil((a+h)*c)-d,!(u*_<=s);)c*=Math.sqrt(s/(u*_));const f=i.map(b=>`${b.x},${b.y}`).join(";");for(const b of[this._warpCache,this._draftWarpCache])if(b&&b.key===f&&b.scale>=c&&b.x<=n&&b.y<=a&&b.x+b.w>=n+o&&b.y+b.h>=a+h)return b;const m=c===1?this._warpCache:this._draftWarpCache,p=c===1?i:i.map(b=>({x:b.x*c,y:b.y*c})),g=m?.canvas??document.createElement("canvas");(g.width!==u||g.height!==_)&&(g.width=u,g.height=_);const y=g.getContext("2d"),w={x:l,y:d,w:u,h:_};Td(this._sourceImageData,p,w,y)||y.putImageData(_d(this._sourceImageData,p,w),0,0);const x={key:f,canvas:g,x:l/c,y:d/c,w:u/c,h:_/c,scale:c};if(c===1){this._warpCache=x;const b=this._draftWarpCache;b&&b.key===f&&x.x<=r.x&&x.y<=r.y&&x.x+x.w>=r.x+r.w&&x.y+x.h>=r.y+r.h&&(b.canvas.width=b.canvas.height=0,this._draftWarpCache=null)}else this._draftWarpCache=x;return x}snapshot(t){const s=this._getSnapshotBounds();if(t){const n=Math.min(s.x+s.w,t.x+t.w),a=Math.min(s.y+s.h,t.y+t.h);if(s.x=Math.max(s.x,t.x),s.y=Math.max(s.y,t.y),s.w=n-s.x,s.h=a-s.y,s.w<=0||s.h<=0)return null}const i=document.createElement("canvas");i.width=s.w,i.height=s.h;const r=i.getContext("2d");return r.save(),r.translate(-s.x,-s.y),this._render(r,t??null,t?1/0:Fd),r.restore(),{canvas:i,...s}}getStateKey(){const t=this._state,s=this._perspectiveActive?vi.map(i=>`${this._perspectiveCorners[i].x},${this._perspectiveCorners[i].y}`).join(";"):"";return`${this._id}:${t.x},${t.y},${t.width},${t.height},${t.rotation},${t.skewX},${t.skewY},${t.scaleX},${t.scaleY}:${s}`}commit(t){const s=t.getContext("2d");if(this._perspectiveActive){const i=this._getWarp({x:0,y:0,w:t.width,h:t.height},1/0);i&&(s.save(),s.setTransform(1,0,0,1,0,0),s.drawImage(i.canvas,i.x,i.y),s.restore())}else{const i=this._matrix();s.save(),s.setTransform(i.a,i.b,i.c,i.d,i.e,i.f),s.drawImage(this._sourceCanvas,0,0,this._state.width,this._state.height),s.restore()}}cancel(){return this._sourceImageData}hasChanged(){const t=this._state,s=this._initialState;return t.x!==s.x||t.y!==s.y||t.width!==s.width||t.height!==s.height||t.rotation!==s.rotation||t.skewX!==s.skewX||t.skewY!==s.skewY||t.scaleX!==s.scaleX||t.scaleY!==s.scaleY||this._perspectiveActive}getState(){return this._state}getSourceRect(){return this._sourceRect}getBounds(){return this._getSnapshotBounds()}updateViewport(t,s){this._zoom=t,this._pan=s,this.renderPreview()}getCursor(t){const s=this._getCorners(),i=this._interaction;switch(i.type){case"moving":return"move";case"rotating":return"grabbing";case"resizing":return Ss(i.handle,s);case"skewing":return Ss(i.edge,s);case"perspective":return Ss(i.corner,s);case"button":return"pointer"}return this.hitTestButton(t)?"pointer":zd(t,this._getCorners(),this._handleConfig,this._zoom)}_onChange(){this.renderPreview()}_matrix(){const t=this._state,s=Gs(t),i=t.rotation/(Math.PI/2),r=Math.abs(t.width*t.scaleX),n=Math.abs(t.height*t.scaleY);if(t.skewX!==0||t.skewY!==0||Math.abs(i-Math.round(i))>1e-9||Math.abs(r-Math.round(r))>1e-9||Math.abs(n-Math.round(n))>1e-9)return s;const a=[s.e,s.a*t.width+s.e,s.c*t.height+s.e,s.a*t.width+s.c*t.height+s.e],o=[s.f,s.b*t.width+s.f,s.d*t.height+s.f,s.b*t.width+s.d*t.height+s.f],h=Math.min(...a),c=Math.min(...o),l=d=>Math.floor(d+.5+1e-7);return s.e+=l(h)-h,s.f+=l(c)-c,s}_getCorners(){if(this._perspectiveActive)return An(this._state,this._perspectiveCorners);const t=this._matrix(),{width:s,height:i}=this._state,r=(n,a)=>({x:t.a*n+t.c*a+t.e,y:t.b*n+t.d*a+t.f});return[r(0,0),r(s,0),r(s,i),r(0,i)]}hitKind(t){const s=this.buttonAt(t);if(s)return s.button;const i=this._getCorners();return Wi(t,i,this._handleConfig,this._zoom)?"rotate":Xi(t,i,this._handleConfig,this._zoom)??(Bs(t,i)?"inside":"outside")}_getSnapshotBounds(){const t=this._getCorners(),s=t.map(h=>h.x),i=t.map(h=>h.y),r=Math.floor(Math.min(...s)+1e-6),n=Math.floor(Math.min(...i)+1e-6),a=Math.max(1,Math.ceil(Math.max(...s)-1e-6)-r),o=Math.max(1,Math.ceil(Math.max(...i)-1e-6)-n);return{x:r,y:n,w:a,h:o}}dispose(){for(const t of[this._warpCache,this._draftWarpCache])t&&(t.canvas.width=t.canvas.height=0);this._warpCache=this._draftWarpCache=null,$d(this._sourceImageData)}}var Xd=Object.getOwnPropertyDescriptor,Wd=(e,t,s,i)=>{for(var r=i>1?void 0:i?Xd(t,s):t,n=e.length-1,a;n>=0;n--)(a=e[n])&&(r=a(r)||r);return r};let Yi=class extends Bt{constructor(){super(...arguments),this._dialog=null,this._resolve=null,this._imgW=0,this._imgH=0,this._canvasW=0,this._canvasH=0}show(e,t,s,i){return new Promise(r=>{this._resolve=r,this._imgW=e,this._imgH=t,this._canvasW=s,this._canvasH=i,this.requestUpdate(),this.updateComplete.then(()=>{this._dialog=this.renderRoot.querySelector("dialog"),this._dialog?.open&&this._dialog.close(),this._dialog?.showModal()})})}_onScale(){this._dialog?.close(),this._resolve?.(!0),this._resolve=null}_onKeep(){this._dialog?.close(),this._resolve?.(!1),this._resolve=null}get asking(){return this._resolve!==null}dismiss(){this._onKeep()}disconnectedCallback(){super.disconnectedCallback(),this._dialog?.open&&this._dialog.close()}connectedCallback(){super.connectedCallback(),this._resolve&&this.updateComplete.then(()=>{this._resolve&&this.isConnected&&this._dialog&&!this._dialog.open&&this._dialog.showModal()})}render(){return v`
      <dialog @cancel=${e=>{e.preventDefault(),this._onKeep()}}>
        <p>
          This image (${this._imgW}&times;${this._imgH}) is larger than the canvas
          (${this._canvasW}&times;${this._canvasH}). Would you like to scale it to fit?
        </p>
        <div class="buttons">
          <button @click=${this._onKeep}>Keep original size</button>
          <button class="primary" @click=${this._onScale}>Scale to fit</button>
        </div>
      </dialog>
    `}};Yi.styles=le`
    dialog {
      background: #2a2a2a;
      color: #e0e0e0;
      border: 1px solid #555;
      border-radius: 8px;
      padding: 24px;
      max-width: 400px;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 14px;
    }
    dialog::backdrop {
      background: rgba(0, 0, 0, 0.5);
    }
    p {
      margin: 0 0 16px;
      line-height: 1.5;
    }
    .buttons {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
    }
    button {
      padding: 8px 16px;
      border-radius: 4px;
      border: 1px solid #555;
      background: #3a3a3a;
      color: #e0e0e0;
      cursor: pointer;
      font-size: 13px;
    }
    button:hover {
      background: #4a4a4a;
    }
    button.primary {
      background: #4a90d9;
      border-color: #4a90d9;
    }
    button.primary:hover {
      background: #5aa0e9;
    }
  `;Yi=Wd([de("resize-dialog")],Yi);var Yd=Object.defineProperty,Ud=Object.getOwnPropertyDescriptor,os=(e,t,s,i)=>{for(var r=i>1?void 0:i?Ud(t,s):t,n=e.length-1,a;n>=0;n--)(a=e[n])&&(r=(i?a(t,s,r):a(r))||r);return i&&r&&Yd(t,s,r),r};function Fn(e){return e.before.width===1&&e.before.height===1&&!Ge(e.before,e.after)}function Vd(e){return(e??"").replace(/[\\/:*?"<>|\u0000-\u001f\u007f]+/g,"-").replace(/\s+/g," ").replace(/^[.\s]+|[.\s]+$/g,"")||"drawing"}function Gd(e,t,s){return{pointerId:e.pointerId,pointerType:e.pointerType,isPrimary:e.isPrimary,button:0,buttons:0,clientX:t,clientY:s,pressure:0,tiltX:e.tiltX,tiltY:e.tiltY,twist:e.twist,width:e.width,height:e.height,timeStamp:e.timeStamp,altKey:e.altKey,ctrlKey:e.ctrlKey,metaKey:e.metaKey,shiftKey:e.shiftKey,preventDefault:()=>e.preventDefault()}}function qd(e){const t=typeof e.getCoalescedEvents=="function"?e.getCoalescedEvents():[];return t.length>0?t:[e]}let q=class extends Bt{constructor(){super(...arguments),this._ctx=new Me(this,{context:Fe,subscribe:!0}),this._checkerboardPattern=null,this._resizeObserver=null,this._lastLayers=null,this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._panX=0,this._panY=0,this._panning=!1,this._panStartX=0,this._panStartY=0,this._panStartOffsetX=0,this._panStartOffsetY=0,this._panPointerId=-1,this._resizePan={x:NaN,y:NaN,dx:0,dy:0},this._moveTempCanvas=null,this._moveStartPoint=null,this._zoom=1,this._pointers=new Map,this._palmPointers=new Set,this._pinchPair="",this._pinching=!1,this._lastPinchDist=0,this._lastPinchMidX=0,this._lastPinchMidY=0,this._transformManager=null,this._transformContentMode="lifted",this._clipboard=null,this._clipboardOrigin=null,this._clipboardRotation=0,this._systemClipboardWrite=null,this._engine=new Na,this._tintPreviewCanvas=null,this._transformViewCanvas=null,this._strokeTintCanvas=null,this._samplingDirty=!0,this._compositeScheduler=As(()=>this._composite(this._compositeContentPending)),this._viewportChangeScheduler=As(()=>this._dispatchViewportChange()),this._viewportChangePending=!1,this._compositeContentPending=!1,this._canvasRect=null,this._strokeTintNeedsClear=!0,this._tintPreviewNeedsCopy=!0,this._samplingBuffer=null,this._altSampling=!1,this._lastPointerScreenX=0,this._lastPointerScreenY=0,this._pointerOnCanvas=!1,this._floatButtonEnd=null,this._pendingTap=null,this._replayingTap=!1,this._floatIsExternalImage=!1,this._selectionDrawing=!1,this._cropRectValue=null,this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._textSelectionBeforePress=null,this._cropRectBeforeNew=null,this._cropActionsVisible=!1,this._textEditing=!1,this._textPosition={x:0,y:0},this._textSelecting=!1,this._textSelectAnchor=0,this._textAreaEl=null,this._textCursorVisible=!1,this._textCursorInterval=0,this._redoHeldByText=!1,this._committingAsPreviewed=!1,this._dprQuery=null,this._onDevicePixelRatioChange=()=>{this.isConnected&&(this._watchDevicePixelRatio(),this._invalidateCanvasRect(),this._resizeToFit())},this._history=[],this._documentGeneration=0,this._historyIndex=-1,this._maxHistory=50,this._historyTrimmed=0,this._layerRevisions=new Map,this._revisionEpoch=0,this._beforeDrawCanvas=null,this._beforeDrawBuffer=null,this._airbrushFrame=0,this._onAirbrushFrame=e=>{this._airbrushFrame=0;const{activeTool:t}=this.ctx.state;!this._drawing||!this._engine.airbrushActive||t!=="pencil"&&t!=="eraser"||(this._engine.tick(e)>0&&this.scheduleComposite(!1),this._airbrushFrame=requestAnimationFrame(this._onAirbrushFrame))},this._floatCommitCache=null,this._redoHeldByFloat=!1,this._invalidateCanvasRect=()=>{this._canvasRect=null},this._onWheel=e=>{if(e.ctrlKey||e.metaKey){if(e.preventDefault(),e.deltaY===0)return;const s=this._getCanvasRect(),i=this._clientScale(s),r=(e.clientX-s.left)*i.x,n=(e.clientY-s.top)*i.y,a=(r-this._panX)/this._zoom,o=(n-this._panY)/this._zoom,h=Math.max(-5,Math.min(5,-e.deltaY*.01)),c=Math.min(q.MAX_ZOOM,Math.max(q.MIN_ZOOM,this._zoom*(1+h)));if(c===this._zoom)return;this._panX=r-a*c,this._panY=n-o*c,this._zoom=c,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(!1),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview(),this._dispatchZoomChange(!0);return}e.preventDefault();const t=this._clientScale(this._getCanvasRect());this._panX-=e.deltaX*t.x,this._panY-=e.deltaY*t.y,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(!1),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview(),this._scheduleViewportChange()},this._laidOut=!1,this._onContextMenu=e=>{this._transformManager&&e.preventDefault()},this._onWindowBlur=()=>{this._stopAirbrush(),this._engine.suspendAirbrush(),this._altSampling=!1,this._clearEyedropperPreview(),this._systemClipboardWrite=null},this._onOtherCopy=()=>{this._systemClipboardWrite=null},this._onPointerEnter=e=>{this._pointerOnCanvas=!0;const t=this._clientToView(e.clientX,e.clientY);this._lastPointerScreenX=t.x,this._lastPointerScreenY=t.y,this._renderPreview()},this._onApplyCropClick=()=>{this.commitCrop()},this._onCancelCropClick=()=>{this.cancelCrop()},this._onDragOver=e=>{e.preventDefault()},this._onDragEnter=e=>{e.preventDefault(),this.classList.add("drop-target")},this._onDragLeave=e=>{e.relatedTarget&&this.contains(e.relatedTarget)||this.classList.remove("drop-target")},this._onDrop=async e=>{if(e.preventDefault(),this.classList.remove("drop-target"),!!e.dataTransfer?.files.length)for(const t of Array.from(e.dataTransfer.files)){if(!t.type.startsWith("image/"))continue;const s=URL.createObjectURL(t),i=t.name.replace(/\.[^.]+$/,"")||"Dropped Image";try{const r=await new Promise((n,a)=>{const o=new Image;o.onload=()=>n(o),o.onerror=()=>a(new Error("Image load failed")),o.src=s});URL.revokeObjectURL(s),await this._handleExternalImage(r,i)}catch{URL.revokeObjectURL(s)}return}}}get ctx(){return this._ctx.value}get _cropRect(){return this._cropRectValue}set _cropRect(e){this._cropRectValue=e,this._updateCropActions()}_updateCropActions(){const e=this._cropRectValue;this._cropActionsVisible=e!==null&&!this._cropDragging&&this._cropHandle===null&&Math.abs(e.w)>=1&&Math.abs(e.h)>=1}get _docWidth(){return this._ctx.value?.state.documentWidth??800}get _docHeight(){return this._ctx.value?.state.documentHeight??600}getWidth(){return this._docWidth}getHeight(){return this._docHeight}invalidateSamplingBuffer(){this._samplingDirty=!0}isGestureActive(){return this._pointers.size>0}scheduleComposite(e=!0){e&&(this._compositeContentPending=!0),this._compositeScheduler.schedule()}isTransformActive(){return this._transformManager!==null}focusText(){return!this._textEditing||!this._textAreaEl?!1:(this._textAreaEl.focus({preventScroll:!0}),!0)}isTextEditing(){return this._textEditing}commitPendingText(){this._textEditing&&this._commitText()}hasPendingText(){return this._textEditing&&!!this._textAreaEl?.value}_dispatchPendingTextChange(){this.dispatchEvent(new CustomEvent("pending-text-change",{detail:{pending:this.hasPendingText()}})),this.hasPendingText()!==this._redoHeldByText&&(this._redoHeldByText=this.hasPendingText(),this._notifyHistory(!1))}enterTransformMode(){if(this._transformManager)return;this._cropRect&&this.cancelCrop();const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(a=>a.id===e.activeLayerId);if(!t)return;const s=t.canvas.getContext("2d"),i=s.getImageData(0,0,t.canvas.width,t.canvas.height),r=Dn(i);if(!r)return;this._captureBeforeDraw();const n=s.getImageData(r.x,r.y,r.w,r.h);s.clearRect(0,0,t.canvas.width,t.canvas.height),this._transformContentMode="lifted",this._transformManager=new ne(n,r,this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}getTransformValues(){if(!this._transformManager)return null;const e=this._transformManager;return{x:e.x,y:e.y,width:e.width,height:e.height,rotation:e.rotation,skewX:e.skewX,skewY:e.skewY,flipH:e.flipH,flipV:e.flipV}}setTransformValue(e,t){if(!this._transformManager)return;const s=this._transformManager;switch(e){case"x":s.x=t;break;case"y":s.y=t;break;case"width":s.width=t;break;case"height":s.height=t;break;case"rotation":s.rotation=t;break;case"skewX":s.skewX=t;break;case"skewY":s.skewY=t;break;case"flipH":s.flipH=!s.flipH;break;case"flipV":s.flipV=!s.flipV;break}this.composite(),this.requestUpdate(),this._dispatchTransformChange()}commitTransform(){if(!this._transformManager)return;const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(a=>a.id===e.activeLayerId);if(!t)return;const s=t.canvas.getContext("2d"),i=this._transformContentMode==="inserted",r=this._floatCommitCache&&this._floatCommitCache.key===this._floatCommitKey()?this._floatCommitCache:null;this._floatCommitCache=null;let n=null;if(!r){if(i){const a=this._transformManager.getBounds(),o=Math.max(0,a.x),h=Math.max(0,a.y),c=Math.min(this._docWidth,a.x+a.w),l=Math.min(this._docHeight,a.y+a.h),d=c-o,u=l-h;d>0&&u>0&&(n={x:o,y:h,w:d,h:u,before:s.getImageData(o,h,d,u)})}}this._transformManager.commit(t.canvas),this._committingAsPreviewed=!!r;try{if(r)r.entry&&this._pushHistoryEntry(r.entry);else if(i&&n){const a=s.getImageData(n.x,n.y,n.w,n.h),o=Ge(n.before,a);o&&this._pushHistoryEntry({type:"patch",layerId:t.id,x:n.x+o.x,y:n.y+o.y,before:Zt(n.before,o),after:Zt(a,o)})}else if(this._transformManager.hasChanged()&&this._beforeDrawCanvas){const a=this._readChangedPatch(s,void 0,!0);a&&this._pushHistoryEntry({type:"patch",layerId:t.id,...a})}this._beforeDrawCanvas=null,this._transformContentMode="lifted",this._floatIsExternalImage=!1,this._transformManager.dispose(),this._transformManager=null,this.previewCanvas.getContext("2d").clearRect(0,0,this.previewCanvas.width,this.previewCanvas.height),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}finally{this._committingAsPreviewed=!1}}cancelTransform(){if(!this._transformManager)return;const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(i=>i.id===e.activeLayerId);if(!t)return;if(this._floatIsExternalImage){this.cancelExternalFloat();return}const s=t.canvas.getContext("2d");if(this._transformContentMode==="inserted")this._transformManager.cancel();else if(this._beforeDrawCanvas)this._restoreBeforeDraw(s);else{const i=this._transformManager.cancel(),r=this._transformManager.getSourceRect();s.putImageData(i,r.x,r.y)}this._transformManager.dispose(),this._transformManager=null,this._floatCommitCache=null,this._beforeDrawCanvas=null,this._transformContentMode="lifted",this.previewCanvas.getContext("2d").clearRect(0,0,this.previewCanvas.width,this.previewCanvas.height),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}get _vw(){return this.mainCanvas?Os(this.mainCanvas).width:800}get _vh(){return this.mainCanvas?Os(this.mainCanvas).height:600}get _brushDescriptor(){const e=this.ctx.state;if(e.brush)return e.brush;const t=Is();return{...t,size:e.brushSize??t.size}}_activeLayer(){const e=this._ctx.value?.state;return e?.layers.find(t=>t.id===e.activeLayerId)}_getActiveLayerCtx(){const e=this._ctx.value?.state;return e?e.layers.find(s=>s.id===e.activeLayerId)?.canvas.getContext("2d")??null:null}composite(){this._composite(!0)}_composite(e){if(!this.mainCanvas)return;this._compositeScheduler.cancel(),this._compositeContentPending=!1;const t=this.mainCanvas.getContext("2d"),s=this._vw;!this._transformManager&&this._transformViewCanvas&&(this._transformViewCanvas.width=this._transformViewCanvas.height=0,this._transformViewCanvas=null);const i=this._vh;t.fillStyle="#3a3a3a",t.fillRect(0,0,s,i),t.save(),t.translate(this._panX,this._panY),t.scale(this._zoom,this._zoom),t.save(),t.beginPath(),t.rect(0,0,this._docWidth,this._docHeight),t.clip();const r=this._getCheckerboardPattern(t);t.fillStyle=r,t.fillRect(0,0,this._docWidth,this._docHeight),t.restore();const n=this._ctx.value?.state.layers??[],a=this._ctx.value?.state.activeLayerId??null,o=n.some(h=>h.visible&&h.blendMode!=="normal");for(const h of n){if(!h.visible)continue;t.globalAlpha=h.opacity;const c=this._transformManager&&h.id===a?this._transformManager:null,l=c!==null&&(h.blendMode!=="normal"||h.opacity<1);o&&(t.globalCompositeOperation=Qe(h.blendMode));const d=this._drawing&&h.id===a?this._engine.getStrokePreview():null,u=d?.bounds??null;if(l)this._drawLayerWithTransformInView(t,h,c);else if(d&&u)if(!d.eraser&&d.mode==="normal"&&!d.alphaLocked&&h.opacity>=1&&h.blendMode==="normal"){t.drawImage(h.canvas,0,0);const f=d.color===null?d.canvas:this._tintStrokeRegion(d.canvas,u,d.color);t.globalAlpha=d.opacity,t.drawImage(f,u.x,u.y,u.w,u.h,u.x,u.y,u.w,u.h),t.globalAlpha=h.opacity}else{(!this._tintPreviewCanvas||this._tintPreviewCanvas.width!==this._docWidth||this._tintPreviewCanvas.height!==this._docHeight)&&(this._tintPreviewCanvas=document.createElement("canvas"),this._tintPreviewCanvas.width=this._docWidth,this._tintPreviewCanvas.height=this._docHeight,this._tintPreviewNeedsCopy=!0);const f=this._tintPreviewCanvas.getContext("2d");f.globalCompositeOperation="source-over";const m=this._engine.takePreviewDelta();let p=u;if(this._tintPreviewNeedsCopy?(f.clearRect(0,0,this._docWidth,this._docHeight),f.drawImage(h.canvas,0,0),this._tintPreviewNeedsCopy=!1):(Aa(d)&&(p=m),p&&(f.clearRect(p.x,p.y,p.w,p.h),f.drawImage(h.canvas,p.x,p.y,p.w,p.h,p.x,p.y,p.w,p.h))),p){const g=d.eraser||d.color===null?d.canvas:this._tintStrokeRegion(d.canvas,p,d.color);La(f,g,p,d)}f.globalAlpha=1,f.globalCompositeOperation="source-over",t.drawImage(this._tintPreviewCanvas,0,0)}else t.drawImage(h.canvas,0,0);if(c)if(l){const _=c.getBounds();_.x>=0&&_.y>=0&&_.x+_.w<=this._docWidth&&_.y+_.h<=this._docHeight||(t.save(),t.beginPath(),t.rect(-1e6,-1e6,2e6,2e6),t.rect(0,0,this._docWidth,this._docHeight),t.clip("evenodd"),c.renderTransformed(t),t.restore())}else c.renderTransformed(t);o&&(t.globalCompositeOperation="source-over"),t.globalAlpha=1}t.strokeStyle="rgba(0,0,0,0.3)",t.lineWidth=1,t.strokeRect(-.5,-.5,this._docWidth+1,this._docHeight+1),t.restore(),this.dispatchEvent(new CustomEvent("composited",{bubbles:!0,composed:!0,detail:{contentChanged:e}})),e&&this.invalidateSamplingBuffer()}_tintStrokeRegion(e,t,s){const i=this._docWidth,r=this._docHeight;(!this._strokeTintCanvas||this._strokeTintCanvas.width!==i||this._strokeTintCanvas.height!==r)&&(this._strokeTintCanvas=document.createElement("canvas"),this._strokeTintCanvas.width=i,this._strokeTintCanvas.height=r,this._strokeTintNeedsClear=!1);const n=this._strokeTintCanvas.getContext("2d");return this._strokeTintNeedsClear&&(n.clearRect(0,0,i,r),this._strokeTintNeedsClear=!1),n.save(),n.beginPath(),n.rect(t.x,t.y,t.w,t.h),n.clip(),n.globalCompositeOperation="source-over",n.clearRect(t.x,t.y,t.w,t.h),n.drawImage(e,t.x,t.y,t.w,t.h,t.x,t.y,t.w,t.h),Mi(n,s,i,r),n.restore(),this._strokeTintCanvas}_getCheckerboardPattern(e){if(!this._checkerboardPattern){const t=document.createElement("canvas");t.width=20,t.height=20;const s=t.getContext("2d");s.fillStyle="#ffffff",s.fillRect(0,0,20,20),s.fillStyle="#e0e0e0",s.fillRect(10,0,10,10),s.fillRect(0,10,10,10),this._checkerboardPattern=e.createPattern(t,"repeat")}return this._checkerboardPattern}willUpdate(){const e=this._ctx.value?.state.layers??null;if(e&&e!==this._lastLayers&&(this._lastLayers=e,this.mainCanvas&&this.composite()),this.mainCanvas&&this._ctx.value&&!this._transformManager){const t=this._ctx.value.state.activeTool;t==="hand"?this.mainCanvas.style.cursor=this._panning?"grabbing":"grab":t==="move"?this.mainCanvas.style.cursor="move":t==="text"?this.mainCanvas.style.cursor="text":this.mainCanvas.style.cursor="crosshair",this._textEditing&&this._renderTextPreview()}this._renderPreview()}firstUpdated(){const e=this._measureView(),t=e?.width??800,s=e?.height??600;this._laidOut=e!==null,this._sizeViewCanvases(t,s,e?.scale??q._devicePixelRatio()),this._zoom=Math.max(q.MIN_ZOOM,Math.min(this._zoom,this._fitZoom())),this._panX=Math.round((t-this._docWidth*this._zoom)/2),this._panY=Math.round((s-this._docHeight*this._zoom)/2),this._observeSize();const i=this._getActiveLayerCtx();i&&(i.fillStyle="#ffffff",i.fillRect(0,0,this._docWidth,this._docHeight)),this.composite(),this._dispatchViewportChange(),this._textAreaEl&&this.shadowRoot.appendChild(this._textAreaEl)}_observeSize(){this._resizeObserver=new ResizeObserver(()=>{this._invalidateCanvasRect(),this._resizeToFit()}),this._resizeObserver.observe(this),this._watchDevicePixelRatio()}static _devicePixelRatio(){const e=typeof window<"u"?window.devicePixelRatio:1;return Number.isFinite(e)&&e>0?e:1}_watchDevicePixelRatio(){if(this._unwatchDevicePixelRatio(),typeof window>"u"||typeof window.matchMedia!="function")return;const e=window.matchMedia(`(resolution: ${q._devicePixelRatio()}dppx)`);e.addEventListener?.("change",this._onDevicePixelRatioChange),this._dprQuery=e}_unwatchDevicePixelRatio(){this._dprQuery?.removeEventListener?.("change",this._onDevicePixelRatioChange),this._dprQuery=null}_measureView(){const e=this.getBoundingClientRect();if(!(e.width>0&&e.height>0))return null;let t=e.width,s=e.height;if(this.isConnected){const n=getComputedStyle(this),a=parseFloat(n.width),o=parseFloat(n.height);a>0&&o>0&&(t=a,s=o)}const i=Math.floor(t),r=Math.floor(s);return i<=0||r<=0?null:{width:i,height:r,scale:q._devicePixelRatio()*(e.width/t)}}_sizeViewCanvases(e,t,s){Hn(this.mainCanvas,e,t,s),Hn(this.previewCanvas,e,t,s)}centerDocument(){this.mainCanvas&&(this._panX=Math.round((this._vw-this._docWidth*this._zoom)/2),this._panY=Math.round((this._vh-this._docHeight*this._zoom)/2),this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview(),this._dispatchViewportChange())}_resizeToFit(){const e=this._measureView();if(!e)return;const t=e.width,s=e.height;this._laidOut=!0;const i=this._vw,r=this._vh,n=i===t&&r===s,a=Ga(t,s,e.scale);if(n&&a.width===this.mainCanvas.width&&a.height===this.mainCanvas.height)return;if(this._sizeViewCanvases(t,s,e.scale),n){this._checkerboardPattern=null,this._redrawView(!1);return}const o=this._panX+(this._panX===this._resizePan.x?this._resizePan.dx:0)+(t-i)/2,h=this._panY+(this._panY===this._resizePan.y?this._resizePan.dy:0)+(s-r)/2;this._panX=Math.round(o),this._panY=Math.round(h),this._resizePan={x:this._panX,y:this._panY,dx:o-this._panX,dy:h-this._panY},this._checkerboardPattern=null,this._redrawView(!0),this._dispatchViewportChange()}_redrawView(e){this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),e?this.composite():this._composite(!1),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview()}getHistory(){return[...this._history]}getHistoryIndex(){return this._historyIndex}getHistoryTrimmedCount(){return this._historyTrimmed}getLayerRevision(e){return`${this._revisionEpoch}:${this._layerRevisions.get(e)??0}`}_bumpLayerRevisions(e){switch(e.type){case"draw":case"patch":case"transform":this._layerRevisions.set(e.layerId,(this._layerRevisions.get(e.layerId)??0)+1);break;case"visibility":case"opacity":case"rename":case"blend-mode":case"alpha-lock":case"reorder":break;default:this._revisionEpoch++}}setHistory(e,t){this._documentGeneration++,this._revisionEpoch++,this._resizeDialog?.dismiss(),this._history=e,this._historyIndex=Math.max(-1,Math.min(t,e.length-1)),this._notifyHistory()}_captureBeforeDraw(){const e=this._getActiveLayerCtx();if(!e)return;const{width:t,height:s}=e.canvas;let i=this._beforeDrawBuffer;(!i||i.width!==t||i.height!==s)&&(i=document.createElement("canvas"),i.width=t,i.height=s,this._beforeDrawBuffer=i);const r=i.getContext("2d");r.clearRect(0,0,t,s),r.drawImage(e.canvas,0,0),this._beforeDrawCanvas=i}_restoreBeforeDraw(e){const t=this._beforeDrawCanvas;t&&(e.save(),e.setTransform(1,0,0,1,0,0),e.globalAlpha=1,e.globalCompositeOperation="source-over",e.clearRect(0,0,e.canvas.width,e.canvas.height),e.drawImage(t,0,0),e.restore())}_pushDrawHistory(e=!1,t){const s=this._ctx.value?.state,i=this._getActiveLayerCtx();if(!i||!s||!this._beforeDrawCanvas)return;const r=this._readChangedPatch(i,t,e);this._beforeDrawCanvas=null,r&&this._pushHistoryEntry({type:"patch",layerId:s.activeLayerId,...r})}_readChangedPatch(e,t,s){const i=this._beforeDrawCanvas,r=Math.min(i.width,e.canvas.width),n=Math.min(i.height,e.canvas.height),a=t??{x:0,y:0,w:r,h:n},o=Math.max(0,Math.floor(a.x)),h=Math.max(0,Math.floor(a.y)),c=Math.min(r,Math.ceil(a.x+a.w))-o,l=Math.min(n,Math.ceil(a.y+a.h))-h,d=i.getContext("2d");let u=null,_=null,f=null;return c>0&&l>0&&(_=d.getImageData(o,h,c,l),f=e.getImageData(o,h,c,l),u=Ge(_,f)),u&&_&&f?{x:o+u.x,y:h+u.y,before:Zt(_,u),after:Zt(f,u)}:!s||r<=0||n<=0?null:{x:0,y:0,before:d.getImageData(0,0,1,1),after:e.getImageData(0,0,1,1)}}_commitStroke(e){if(this._stopAirbrush(),!this._engine.commit(e))return;const t=this._engine.getDirtyBounds();if(!t)return{x:0,y:0,w:0,h:0};const s=2;return{x:t.x-s,y:t.y-s,w:t.w+s*2,h:t.h+s*2}}_startAirbrush(){this._airbrushFrame||!this._drawing||!this._engine.airbrushActive||(this._airbrushFrame=requestAnimationFrame(this._onAirbrushFrame))}_stopAirbrush(){this._airbrushFrame&&(cancelAnimationFrame(this._airbrushFrame),this._airbrushFrame=0)}pushLayerOperation(e){this._pushHistoryEntry(e)}_pushHistoryEntry(e){const{history:t,trimmed:s}=this._historyWith(e);this._history=t,this._bumpLayerRevisions(e),this._historyIndex=t.length-1,this._historyTrimmed+=s,this._notifyHistory()}_historyWith(e){const t=this._history.slice(0,this._historyIndex+1);t.push(e);let s=t.reduce((n,a)=>n+Wr(a),0);const i=Ma();let r=0;for(;t.length>1&&(t.length>this._maxHistory||s>i);)s-=Wr(t.shift()),r++;return{history:t,trimmed:r}}_floatCommitKey(){const e=this._ctx.value?.state.activeLayerId;return!this._transformManager||!e?null:`${e}|${this.getLayerRevision(e)}|${this._docWidth}x${this._docHeight}|${this._transformContentMode}|${this._transformManager.getStateKey()}`}_floatCommitEntry(e,t){const s=this._transformManager;if(this._transformContentMode==="inserted"){const c=s.getBounds(),l=Math.max(0,c.x),d=Math.max(0,c.y),u=Math.min(this._docWidth,c.x+c.w)-l,_=Math.min(this._docHeight,c.y+c.h)-d;if(u<=0||_<=0)return null;const f=e.canvas.getContext("2d").getImageData(l,d,u,_),m=Zt(t,{x:l,y:d,w:u,h:_}),p=Ge(f,m);return p?{type:"patch",layerId:e.id,x:l+p.x,y:d+p.y,before:Zt(f,p),after:Zt(m,p)}:null}const i=this._beforeDrawCanvas;if(!s.hasChanged()||!i)return null;const r=Math.min(i.width,t.width),n=Math.min(i.height,t.height);if(r<=0||n<=0)return null;const a=i.getContext("2d").getImageData(0,0,r,n),o=Zt(t,{x:0,y:0,w:r,h:n}),h=Ge(a,o)??{x:0,y:0,w:1,h:1};return{type:"patch",layerId:e.id,x:h.x,y:h.y,before:Zt(a,h),after:Zt(o,h)}}previewFloatCommit(){const e=this._floatCommitKey(),t=this._ctx.value?.state,s=t?.layers.find(o=>o.id===t.activeLayerId);if(!e||!s)return null;const i=document.createElement("canvas");i.width=s.canvas.width,i.height=s.canvas.height;const r=i.getContext("2d");r.drawImage(s.canvas,0,0),this._transformManager.commit(i);const n=r.getImageData(0,0,i.width,i.height);i.width=i.height=0;const a=this._floatCommitCache?.key===e?this._floatCommitCache.entry:this._floatCommitEntry(s,n);return this._floatCommitCache={key:e,entry:a},{layerId:s.id,imageData:n,entry:a}}getHistoryAsCommitted(){const e=this._floatCommitKey();let t;if(e&&(t=this._floatCommitCache?.key===e?this._floatCommitCache.entry:this.previewFloatCommit()?.entry),!t)return{history:this.getHistory(),index:this._historyIndex};const{history:s}=this._historyWith(t);return{history:s,index:s.length-1}}_getEntryLayerId(e){switch(e.type){case"draw":case"patch":case"visibility":case"opacity":case"rename":case"blend-mode":case"alpha-lock":case"transform":return e.layerId;case"add-layer":case"delete-layer":return e.layer.id;case"reorder":return null;case"crop":case"merge":return null}}_notifyHistory(e=!0){this.dispatchEvent(new CustomEvent("history-change",{bubbles:!0,composed:!0,detail:{canUndo:this._historyIndex>=0||this._transformManager!==null,canRedo:this._historyIndex<this._history.length-1&&!this.hasPendingText()&&!this._floatHoldsWork(),stackChanged:e,committedAsPreviewed:this._committingAsPreviewed}}))}_floatHoldsWork(){const e=this._transformManager;return!!e&&(this._transformContentMode==="inserted"||e.hasChanged())}_dispatchTransformChange(){this._floatHoldsWork()!==this._redoHeldByFloat&&(this._redoHeldByFloat=this._floatHoldsWork(),this._notifyHistory(!1)),this.dispatchEvent(new CustomEvent("transform-change",{bubbles:!0,composed:!0,detail:{active:this._transformManager!==null,values:this.getTransformValues()}}))}_transformValuesEqual(e,t){return e===t?!0:!e||!t?!1:e.x===t.x&&e.y===t.y&&e.width===t.width&&e.height===t.height&&e.rotation===t.rotation&&e.skewX===t.skewX&&e.skewY===t.skewY&&e.flipH===t.flipH&&e.flipV===t.flipV}undo(){if(this._textEditing&&this._commitText(),this._drawing){const t=this._getActiveLayerCtx(),s=t?this._commitStroke(t):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null;const i=this._beforeDrawCanvas!==null;if(this._pushDrawHistory(!1,s),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),!i)return}if(this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._transformManager){const t=this._history;if(this.commitTransform(),this._history===t)return}if(this._historyIndex<0)return;const e=this._history[this._historyIndex];this._historyIndex--,this._applyUndo(e),this._bumpLayerRevisions(e),e.type==="crop"&&this._cropRect&&this.cancelCrop(),this.composite(),this._notifyHistory()}redo(){if(this.hasPendingText()||this._floatHoldsWork())return;if(this._textEditing&&this._commitText(),this._drawing){const t=this._getActiveLayerCtx(),s=t?this._commitStroke(t):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null;const i=this._beforeDrawCanvas!==null;if(this._pushDrawHistory(!1,s),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),!i)return}if(this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._historyIndex>=this._history.length-1)return;this._transformManager&&this.cancelTransform(),this._historyIndex++;const e=this._history[this._historyIndex];this._applyRedo(e),this._bumpLayerRevisions(e),e.type==="crop"&&this._cropRect&&this.cancelCrop(),this.composite(),this._notifyHistory()}_applyUndo(e){const t=this._ctx.value?.state;if(t)switch(e.type){case"draw":case"transform":{const s=t.layers.find(i=>i.id===e.layerId);s&&s.canvas.getContext("2d").putImageData(e.before,0,0);break}case"patch":{const s=t.layers.find(i=>i.id===e.layerId);s&&!Fn(e)&&s.canvas.getContext("2d").putImageData(e.before,e.x,e.y);break}case"add-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:e.layer.id}}));break}case"delete-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"restore-layer",snapshot:e.layer,index:e.index}}));break}case"reorder":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"reorder",fromIndex:e.toIndex,toIndex:e.fromIndex}}));break}case"visibility":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.visible=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"opacity":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.opacity=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"rename":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.name=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"blend-mode":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.blendMode=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"alpha-lock":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.alphaLocked=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"crop":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"crop-restore",layers:e.beforeLayers,width:e.beforeWidth,height:e.beforeHeight}}));break}case"merge":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"stack-replace",layers:e.beforeLayers,activeLayerId:e.previousActiveLayerId}}));break}}}_applyRedo(e){const t=this._ctx.value?.state;if(t)switch(e.type){case"draw":case"transform":{const s=t.layers.find(i=>i.id===e.layerId);s&&s.canvas.getContext("2d").putImageData(e.after,0,0);break}case"patch":{const s=t.layers.find(i=>i.id===e.layerId);s&&!Fn(e)&&s.canvas.getContext("2d").putImageData(e.after,e.x,e.y);break}case"add-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"restore-layer",snapshot:e.layer,index:e.index}}));break}case"delete-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:e.layer.id}}));break}case"reorder":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"reorder",fromIndex:e.fromIndex,toIndex:e.toIndex}}));break}case"visibility":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.visible=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"opacity":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.opacity=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"rename":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.name=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"blend-mode":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.blendMode=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"alpha-lock":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.alphaLocked=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"crop":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"crop-restore",layers:e.afterLayers,width:e.afterWidth,height:e.afterHeight}}));break}case"merge":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"stack-replace",layers:e.afterLayers,activeLayerId:e.afterActiveLayerId}}));break}}}clearCanvas(){if(this._drawing){const t=this._getActiveLayerCtx(),s=t?this._commitStroke(t):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!0,s)}this.clearSelection(),this._captureBeforeDraw();const e=this._getActiveLayerCtx();e&&e.clearRect(0,0,this._docWidth,this._docHeight),this._pushDrawHistory(!0),this.composite()}_drawLayerWithTransformInView(e,t,s){const{width:i,height:r}=e.canvas;let n=this._transformViewCanvas;n||(n=this._transformViewCanvas=document.createElement("canvas")),(n.width!==i||n.height!==r)&&(n.width=i,n.height=r);const a=e.getTransform(),o=t.canvas.width,h=t.canvas.height,c=[a.e,a.a*o+a.e,a.c*h+a.e,a.a*o+a.c*h+a.e],l=[a.f,a.b*o+a.f,a.d*h+a.f,a.b*o+a.d*h+a.f],d=Math.max(0,Math.floor(Math.min(...c))),u=Math.max(0,Math.floor(Math.min(...l))),_=Math.min(i,Math.ceil(Math.max(...c)))-d,f=Math.min(r,Math.ceil(Math.max(...l)))-u;if(_<=0||f<=0)return;const m=n.getContext("2d");m.setTransform(1,0,0,1,0,0),m.clearRect(d,u,_,f),m.setTransform(a),m.save(),m.beginPath(),m.rect(0,0,o,h),m.clip(),m.drawImage(t.canvas,0,0),s.renderTransformed(m),m.restore(),e.save(),e.setTransform(1,0,0,1,0,0),e.drawImage(n,d,u,_,f,d,u,_,f),e.restore()}_layerWithTransform(e,t){if(!this._transformManager||e.id!==t)return e.canvas;const s=document.createElement("canvas");s.width=e.canvas.width,s.height=e.canvas.height;const i=s.getContext("2d");return i.drawImage(e.canvas,0,0),this._transformManager.renderTransformed(i,{x:0,y:0,w:s.width,h:s.height}),s}renderFlattened(e="#ffffff"){const t=document.createElement("canvas");t.width=this._docWidth,t.height=this._docHeight;const s=t.getContext("2d");e&&(s.fillStyle=e,s.fillRect(0,0,this._docWidth,this._docHeight));const i=this._ctx.value?.state,r=i?.layers??[],n=i?.activeLayerId??null;for(const a of r)a.visible&&(s.globalAlpha=a.opacity,s.globalCompositeOperation=Qe(a.blendMode),s.drawImage(this._layerWithTransform(a,n),0,0),s.globalCompositeOperation="source-over",s.globalAlpha=1);return t}saveCanvas(){const e=this.renderFlattened("#ffffff"),t=document.createElement("a");t.download=`${Vd(this._ctx.value?.currentProject?.name)}.png`,t.href=e.toDataURL("image/png"),t.click()}_getCanvasRect(){return this._canvasRect||(this._canvasRect=this.mainCanvas.getBoundingClientRect()),this._canvasRect}_getDocPoint(e){const t=this._getCanvasRect(),s=this._clientScale(t);return{x:((e.clientX-t.left)*s.x-this._panX)/this._zoom,y:((e.clientY-t.top)*s.y-this._panY)/this._zoom}}_clientScale(e){const t=this._vw,s=this._vh;return{x:e.width>0&&t>0?t/e.width:1,y:e.height>0&&s>0?s/e.height:1}}_clientToView(e,t){const s=this._getCanvasRect(),i=this._clientScale(s);return{x:(e-s.left)*i.x,y:(t-s.top)*i.y}}_clientToDoc(e,t){const s=this._getCanvasRect(),i=this._clientScale(s);return{x:((e-s.left)*i.x-this._panX)/this._zoom,y:((t-s.top)*i.y-this._panY)/this._zoom}}_startPan(e){this._panning=!0,this._panStartX=e.clientX,this._panStartY=e.clientY,this._panStartOffsetX=this._panX,this._panStartOffsetY=this._panY,this._panPointerId=e.pointerId,this.mainCanvas.setPointerCapture(e.pointerId),this.mainCanvas.style.cursor="grabbing"}_updatePan(e){if(!this._panning)return;const t=this._clientScale(this._getCanvasRect());this._panX=this._panStartOffsetX+(e.clientX-this._panStartX)*t.x,this._panY=this._panStartOffsetY+(e.clientY-this._panStartY)*t.y,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(!1),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview()}_endPan(){if(!this._panning)return;const e=this._panPointerId;if(this._panning=!1,this._panPointerId=-1,e>=0&&this.mainCanvas)try{this.mainCanvas.releasePointerCapture(e)}catch{}if(this._ctx.value){const t=this._ctx.value.state.activeTool;t==="hand"?this.mainCanvas.style.cursor="grab":t==="move"?this.mainCanvas.style.cursor="move":this.mainCanvas.style.cursor="crosshair"}this._dispatchViewportChange()}_dispatchZoomChange(e=!1){this.dispatchEvent(new CustomEvent("zoom-change",{bubbles:!0,composed:!0,detail:{zoom:this._zoom}})),e?this._scheduleViewportChange():this._dispatchViewportChange()}_scheduleViewportChange(){this._viewportChangePending=!0,this._viewportChangeScheduler.schedule()}flushViewportChange(){this._viewportChangePending&&this._dispatchViewportChange()}_dispatchViewportChange(){this._viewportChangeScheduler.cancel(),this._viewportChangePending=!1,this.dispatchEvent(new CustomEvent("viewport-change",{bubbles:!0,composed:!0}))}zoomIn(){this._zoomToCenter(this._zoom*q.ZOOM_STEP)}zoomOut(){this._zoomToCenter(this._zoom/q.ZOOM_STEP)}resetView(){this.mainCanvas&&this._setCenteredZoom(Math.min(1,this._fitZoom()))}zoomToFit(){this._setCenteredZoom(this._fitZoom())}_fitZoom(){return Math.min(this._vw/this._docWidth,this._vh/this._docHeight)*.9}_setCenteredZoom(e){this._zoom=Math.min(q.MAX_ZOOM,Math.max(q.MIN_ZOOM,e)),this._panX=Math.round((this._vw-this._docWidth*this._zoom)/2),this._panY=Math.round((this._vh-this._docHeight*this._zoom)/2),this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview(),this._dispatchZoomChange()}getZoom(){return this._zoom}getViewport(){return{zoom:this._zoom,panX:this._panX,panY:this._panY}}getViewportSize(){return this._laidOut?{width:this._vw,height:this._vh}:null}restoreViewport(e,t,s,i){if(this._resizeToFit(),i&&this._laidOut){if(!q._similarSize(i,{width:this._vw,height:this._vh})){this.resetView();return}const r=(this._vw-i.width)/2,n=(this._vh-i.height)/2,a=t+r,o=s+n;t=Math.round(t+Math.trunc(r)),s=Math.round(s+Math.trunc(n)),this.setViewport(e,t,s),this._resizePan={x:t,y:s,dx:a-t,dy:o-s}}else this.setViewport(e,t,s);this._visibleDocumentFraction()<.5&&this.resetView()}static _similarSize(e,t){const s=(i,r)=>Math.abs(i-r)<=Math.max(i,r)*.2;return s(e.width,t.width)&&s(e.height,t.height)}_visibleDocumentFraction(){const e=this._docWidth*this._zoom,t=this._docHeight*this._zoom,s=Math.max(0,Math.min(this._vw,this._panX+e)-Math.max(0,this._panX)),i=Math.max(0,Math.min(this._vh,this._panY+t)-Math.max(0,this._panY)),r=Math.min(e,this._vw)*Math.min(t,this._vh);return r>0?s*i/r:0}setViewport(e,t,s){this._zoom=Math.min(q.MAX_ZOOM,Math.max(q.MIN_ZOOM,e)),this._panX=t,this._panY=s,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(!1),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview(),this._dispatchViewportChange()}_zoomToCenter(e){const t=Math.min(q.MAX_ZOOM,Math.max(q.MIN_ZOOM,e));if(t===this._zoom)return;const s=this._vw/2,i=this._vh/2,r=(s-this._panX)/this._zoom,n=(i-this._panY)/this._zoom;this._panX=s-r*t,this._panY=i-n*t,this._zoom=t,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview(),this._dispatchZoomChange()}_ensureSamplingBuffer(){(!this._samplingBuffer||this._samplingBuffer.width!==this._docWidth||this._samplingBuffer.height!==this._docHeight)&&(this._samplingBuffer=document.createElement("canvas"),this._samplingBuffer.width=this._docWidth,this._samplingBuffer.height=this._docHeight,this._samplingDirty=!0);const e=this._samplingBuffer.getContext("2d",{willReadFrequently:!0});if(this._samplingDirty){e.clearRect(0,0,this._docWidth,this._docHeight),e.fillStyle="#ffffff",e.fillRect(0,0,this._docWidth,this._docHeight);const t=this._ctx.value?.state.layers??[],s=this._ctx.value?.state.activeLayerId??null;for(const i of t)i.visible&&(e.globalAlpha=i.opacity,e.globalCompositeOperation=Qe(i.blendMode),e.drawImage(this._layerWithTransform(i,s),0,0),e.globalCompositeOperation="source-over");e.globalAlpha=1,this._samplingDirty=!1}return e}_sampleColor(e,t){const s=Math.round(e),i=Math.round(t);if(s<0||i<0||s>=this._docWidth||i>=this._docHeight)return null;if(this.ctx.state.eyedropperSampleAll){const a=this._ensureSamplingBuffer().getImageData(s,i,1,1).data;return`#${a[0].toString(16).padStart(2,"0")}${a[1].toString(16).padStart(2,"0")}${a[2].toString(16).padStart(2,"0")}`}else{const n=this._getActiveLayerCtx();if(!n)return null;const a=n.getImageData(s,i,1,1).data;if(a[3]===0)return null;if(a[3]<255){const o=a[3]/255,h=Math.round(a[0]*o+255*(1-o)),c=Math.round(a[1]*o+255*(1-o)),l=Math.round(a[2]*o+255*(1-o));return`#${h.toString(16).padStart(2,"0")}${c.toString(16).padStart(2,"0")}${l.toString(16).padStart(2,"0")}`}return`#${a[0].toString(16).padStart(2,"0")}${a[1].toString(16).padStart(2,"0")}${a[2].toString(16).padStart(2,"0")}`}}_renderEyedropperPreview(e){const t=this.previewCanvas.getContext("2d");t.clearRect(0,0,this._vw,this._vh);const s=this._getDocPoint(e),i=this._sampleColor(s.x,s.y),n=this.ctx.state.eyedropperSampleAll?this._ensureSamplingBuffer().canvas:this._getActiveLayerCtx()?.canvas,a=88,o=24,h=a+o+4,c=20,l=this._clientToView(e.clientX,e.clientY);let d=l.x+c,u=l.y-c-h;if(d+a>this._vw&&(d=d-a-2*c),u<0&&(u=u+h+2*c),t.save(),t.imageSmoothingEnabled=!1,n){const p=Math.round(s.x),g=Math.round(s.y);t.drawImage(n,p-5,g-5,11,11,d,u,a,a)}t.restore(),t.strokeStyle="rgba(255,255,255,0.3)",t.lineWidth=.5;const _=a/11;for(let p=0;p<=11;p++){const g=d+p*_,y=u+p*_;t.beginPath(),t.moveTo(g,u),t.lineTo(g,u+a),t.stroke(),t.beginPath(),t.moveTo(d,y),t.lineTo(d+a,y),t.stroke()}const f=d+5*_,m=u+5*_;t.strokeStyle="#fff",t.lineWidth=1.5,t.strokeRect(f,m,_,_),t.strokeStyle="#555",t.lineWidth=1,t.strokeRect(d-.5,u-.5,a+1,h+1),i&&(t.fillStyle=i,t.fillRect(d,u+a+2,o,o),t.fillStyle="#fff",t.font="11px monospace",t.fillText(i.toUpperCase(),d+o+6,u+a+16))}_clearEyedropperPreview(){const e=this.previewCanvas?.getContext("2d");e&&e.clearRect(0,0,this._vw,this._vh)}_blurFocusedField(){let e=document.activeElement;for(;e?.shadowRoot?.activeElement;)e=e.shadowRoot.activeElement;e!==this._textAreaEl&&(e instanceof HTMLInputElement||e instanceof HTMLSelectElement||e instanceof HTMLTextAreaElement)&&(this._textEditing?this._textAreaEl?.focus():ee(this),e.blur())}_isInTextBox(e){const t=this._getTextBoundingBox();return e.x>=t.x&&e.x<=t.x+t.w&&e.y>=t.y&&e.y<=t.y+t.h}_renderBrushCursor(){if(!this._pointerOnCanvas||this._altSampling)return;const{activeTool:e}=this.ctx.state,t=this._brushDescriptor,s=t.size,i=t.hardness;if(e!=="pencil"&&e!=="eraser")return;const r=this.previewCanvas.getContext("2d"),n=this._lastPointerScreenX,a=this._lastPointerScreenY,o=s/2*this._zoom;if(r.beginPath(),r.arc(n,a,o,0,Math.PI*2),r.strokeStyle="rgba(0,0,0,0.7)",r.lineWidth=1.5,r.stroke(),r.beginPath(),r.arc(n,a,o,0,Math.PI*2),r.strokeStyle="rgba(255,255,255,0.7)",r.lineWidth=.75,r.stroke(),i<1&&t.tip.shape!=="image"){const h=o*i;r.beginPath(),r.arc(n,a,h,0,Math.PI*2),r.setLineDash([3,3]),r.strokeStyle="rgba(255,255,255,0.5)",r.lineWidth=.75,r.stroke(),r.setLineDash([])}}_renderStampCursor(){if(!this._pointerOnCanvas||this._transformManager)return;const e=this.ctx.state.stampImage;if(!e||e.naturalWidth<=0||e.naturalHeight<=0)return;const t=this.previewCanvas.getContext("2d"),i=this.ctx.state.stampSize/Math.max(e.naturalWidth,e.naturalHeight),r=Math.max(1,e.naturalWidth*i)*this._zoom,n=Math.max(1,e.naturalHeight*i)*this._zoom,a=this._lastPointerScreenX-r/2,o=this._lastPointerScreenY-n/2;t.save(),t.globalAlpha=.55,t.drawImage(e,a,o,r,n),t.globalAlpha=1,t.strokeStyle="rgba(255,255,255,0.9)",t.lineWidth=1,t.setLineDash([4,3]),t.strokeRect(a-.5,o-.5,r+1,n+1),t.restore()}_renderPreview(){const e=this.previewCanvas?.getContext("2d");if(!e)return;if(this._transformManager){this._transformManager.renderPreview();return}const{activeTool:t}=this.ctx.state;if(t==="pencil"||t==="eraser"||t==="eyedropper"||t==="stamp"){if(e.clearRect(0,0,this._vw,this._vh),this._altSampling||t==="eyedropper")return;t==="stamp"?this._renderStampCursor():this._drawing||this._renderBrushCursor()}}_onPointerDown(e){if(this._invalidateCanvasRect(),!this._replayingTap){if(e.pointerType==="touch"&&[...this._pointers.values()].some(a=>a.type==="pen")){this._palmPointers.add(e.pointerId);return}if(e.pointerType==="pen"){for(const[a,o]of this._pointers)o.type==="touch"&&(this._cancelCurrentTool(a,!0),this._pointers.delete(a),this._palmPointers.add(a));this._pendingTap=null,this._pinching=!1}if(e.isPrimary){for(const[a,o]of this._pointers)a!==e.pointerId&&o.type===e.pointerType&&this._pointers.delete(a);this._pointers.size<2&&(this._pinching=!1)}if(e.button===0){const a=this._transformManager,o=this._getDocPoint(e),h=a&&{kind:a.hitKind(o),button:a.buttonAt(o),values:this.getTransformValues()};if(this._blurFocusedField(),a&&h&&a===this._transformManager&&(h.kind!==a.hitKind(o)||h.kind==="outside"&&!this._transformValuesEqual(h.values,this.getTransformValues()))){this._pointers.set(e.pointerId,{x:e.clientX,y:e.clientY,type:e.pointerType}),h.button&&a.pressButton(h.button),this.mainCanvas.setPointerCapture(e.pointerId);return}}}if(this._pointers.set(e.pointerId,{x:e.clientX,y:e.clientY,type:e.pointerType}),this._pointers.size===2){this._enterPinchMode(e);return}if(this._pointers.size>2||!this._ctx.value)return;if(e.button===1){e.preventDefault(),this._startPan(e);return}if(e.button!==0){this._pointers.delete(e.pointerId);return}const t=this._transformManager,s=this.ctx.state.activeTool==="hand";if(t&&e.pointerType==="touch"&&!t.touchMode&&!t.hitTestButton(this._getDocPoint(e))){const a=this._getDocPoint(e),o=t.hitKind(a);if(t.setTouchMode(!0),s?t.hitTestButton(a):t.hitKind(a)!==o)return}if(t&&s&&!t.hitTestButton(this._getDocPoint(e))){this._startPan(e);return}if(t){const a=this._getDocPoint(e),o={shift:e.shiftKey,ctrl:e.ctrlKey||e.metaKey,alt:e.altKey};t.onPointerDown(a,o,e.pointerType==="pen"?zs.outsideDragThreshold:void 0),this._replayingTap||this.mainCanvas.setPointerCapture(e.pointerId);return}const{activeTool:i}=this.ctx.state;if(e.pointerType==="touch"&&!this._replayingTap&&(i==="stamp"||i==="fill"||i==="eyedropper"||i==="text"&&(!this._textEditing||!this._isInTextBox(this._getDocPoint(e))))){if(i==="text"&&e.preventDefault(),i==="stamp"){const a=this._floatButtonEnd;if(this._floatButtonEnd=null,a&&e.timeStamp-a.time<400&&Math.hypot(e.clientX-a.x,e.clientY-a.y)<6)return}this._pendingTap={pointerId:e.pointerId,down:e};return}if(e.altKey&&(i==="pencil"||i==="eraser")){this._altSampling=!0;const a=this._getDocPoint(e),o=this._sampleColor(a.x,a.y);o&&this.ctx.setStrokeColor(o);return}if(i==="hand"){this._startPan(e);return}if(i==="crop"){this.mainCanvas.setPointerCapture(e.pointerId);const a=this._getDocPoint(e);this._handleCropPointerDown(a,e.pointerType==="touch");return}if(i==="eyedropper"){const a=this._getDocPoint(e),o=this._sampleColor(a.x,a.y);o&&this.ctx.setStrokeColor(o);return}const r=this.ctx.state.layers.find(a=>a.id===this.ctx.state.activeLayerId);if(r&&!r.visible)return;if(i==="move"){this.mainCanvas.setPointerCapture(e.pointerId),this._transformManager&&this.commitTransform();const a=this._getDocPoint(e);this._captureBeforeDraw();const o=this._getActiveLayerCtx();if(!o)return;const h=document.createElement("canvas");h.width=this._docWidth,h.height=this._docHeight,h.getContext("2d").drawImage(o.canvas,0,0),this._moveTempCanvas=h,this._moveStartPoint=a;return}this._replayingTap||this.mainCanvas.setPointerCapture(e.pointerId);const n=this._getDocPoint(e);if(i==="select"){this._handleSelectPointerDown(n);return}if(i==="fill"){const a=Math.round(n.x),o=Math.round(n.y);if(a>=0&&o>=0&&a<this._docWidth&&o<this._docHeight){const h=this._getActiveLayerCtx();if(h){this._captureBeforeDraw(),this._beforeDrawCanvas?.getContext("2d").getImageData(0,0,1,1);const c=ad(h,a,o,this.ctx.state.strokeColor,void 0,this._activeLayer()?.alphaLocked===!0);c?(this._pushDrawHistory(!1,c),this.composite()):this._beforeDrawCanvas=null}}return}if(i==="stamp"){const a=this._floatButtonEnd;if(this._floatButtonEnd=null,a&&e.timeStamp-a.time<400&&Math.hypot(e.clientX-a.x,e.clientY-a.y)<6)return;this._transformManager&&this.commitTransform(),this.ctx.state.stampImage&&(this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this._createStampAsTransform(this.ctx.state.stampImage,n.x,n.y,this.ctx.state.stampSize,e.pointerType==="touch"));return}if(i==="text"){if(e.preventDefault(),this._textEditing){const a=this._getTextBoundingBox();if(n.x>=a.x&&n.x<=a.x+a.w&&n.y>=a.y&&n.y<=a.y+a.h){const o=this._pointToTextOffset(n);this._textAreaEl&&(this._textSelectionBeforePress=[this._textAreaEl.selectionStart,this._textAreaEl.selectionEnd],this._textAreaEl.selectionStart=o,this._textAreaEl.selectionEnd=o),this._textSelectAnchor=o,this._textSelecting=!0,this._startTextCursorBlink(),this._renderTextPreview();return}this._commitText();return}this._textPosition=n,this._textEditing=!0,this._textAreaEl&&(this._textAreaEl.value="",this._textAreaEl.focus()),this._startTextCursorBlink(),this._renderTextPreview();return}if(this._drawing=!0,this._lastPoint=n,this._startPoint=n,i==="pencil"||i==="eraser"){this.previewCanvas?.getContext("2d")?.clearRect(0,0,this._vw,this._vh),this._captureBeforeDraw();const a=this._brushDescriptor,o=this.ctx.state.strokeColor,h=this.ctx.state.activeTool==="eraser";this._engine.begin(a,o,h,this._docWidth,this._docHeight,{stabilizerMs:Hc(this.ctx.state.stabilizer),secondaryColor:this.ctx.state.fillColor,alphaLocked:this._activeLayer()?.alphaLocked===!0,viewScale:this._zoom/this._clientScale(this._getCanvasRect()).x}),this._strokeTintNeedsClear=!0,this._tintPreviewNeedsCopy=!0;const c=a.ink.wetness>0?this._getActiveLayerCtx()??void 0:void 0;this._engine.stroke(n.x,n.y,Mn(e),c,e.timeStamp,{...Cn(e),twist:Sn(e)}),this._startAirbrush(),this.composite()}}_onPointerMove(e){if(this._palmPointers.has(e.pointerId))return;const t=this._pointers.get(e.pointerId);if(e.pointerType==="mouse"&&e.buttons===0&&t){this._onPointerUp(Gd(e,t.x,t.y));return}if(!t&&this._pointers.size>0)return;t&&this._pointers.set(e.pointerId,{x:e.clientX,y:e.clientY,type:t.type});const s=this._clientToView(e.clientX,e.clientY);if(this._lastPointerScreenX=s.x,this._lastPointerScreenY=s.y,this._pinching){this._updatePinch();return}if(!this._ctx.value)return;if(this._panning){this._updatePan(e);return}if(this._transformManager){const n=this._getDocPoint(e),a={shift:e.shiftKey,ctrl:e.ctrlKey||e.metaKey,alt:e.altKey},o=this.getTransformValues(),h=this._transformManager.onPointerMove(n,a);if(this.mainCanvas.style.cursor=this._transformCursor(n),h){this.invalidateSamplingBuffer(),this.scheduleComposite(!1);const c=this.getTransformValues();this._transformValuesEqual(o,c)||this._dispatchTransformChange()}return}{const n=this.ctx.state.activeTool,a=()=>{if(!t||e.pointerType==="touch"||!(e.buttons&1)||this._drawing)return;const o=this._getDocPoint(e),h=this._sampleColor(o.x,o.y);h&&this.ctx.setStrokeColor(h)};if(n==="eyedropper"){this._renderEyedropperPreview(e),a();return}if(this._altSampling||e.altKey&&(n==="pencil"||n==="eraser")){if(this._altSampling=e.altKey,!e.altKey){this._clearEyedropperPreview();return}this._renderEyedropperPreview(e),a();return}}if(this._textSelecting){const n=this._getDocPoint(e),a=this._pointToTextOffset(n);if(this._textAreaEl){const o=this._textSelectAnchor;this._textAreaEl.selectionStart=Math.min(o,a),this._textAreaEl.selectionEnd=Math.max(o,a)}this._renderTextPreview();return}const{activeTool:i}=this.ctx.state;if(i==="crop"){this._handleCropPointerMove(e);return}if(i==="move"&&this._moveTempCanvas&&this._moveStartPoint){const n=this._getDocPoint(e);let a=n.x-this._moveStartPoint.x,o=n.y-this._moveStartPoint.y;e.shiftKey&&(Math.abs(a)>Math.abs(o)?o=0:a=0);const h=this._getActiveLayerCtx();h&&(h.clearRect(0,0,this._docWidth,this._docHeight),h.drawImage(this._moveTempCanvas,Math.round(a),Math.round(o)),this.scheduleComposite());return}if(i==="select"){this._handleSelectPointerMove(e);return}if(i==="stamp"){this._renderPreview();return}if(!this._drawing&&(i==="pencil"||i==="eraser")&&this._renderPreview(),!this._drawing||!this._lastPoint)return;const r=this._getDocPoint(e);if(i==="pencil"||i==="eraser"){const a=this._brushDescriptor.ink.wetness>0?this._getActiveLayerCtx()??void 0:void 0;for(const o of qd(e)){const h=o===e?r:this._getDocPoint(o);this._engine.stroke(h.x,h.y,Mn(o),a,o.timeStamp,{...Cn(o),twist:Sn(o)})}this._lastPoint=r,this._startAirbrush(),this.scheduleComposite(!1)}else if(ce(i)){const n=this.previewCanvas.getContext("2d");n.clearRect(0,0,this._vw,this._vh),n.save(),n.translate(this._panX,this._panY),n.scale(this._zoom,this._zoom),Kr(n,i,this._startPoint,r,this.ctx.state.strokeColor,this.ctx.state.fillColor,this.ctx.state.useFill,this._brushDescriptor.size),n.restore()}}_refreshTransformCursor(e){this._transformManager&&(this.mainCanvas.style.cursor=this._transformCursor(this._getDocPoint(e)))}_transformCursor(e){const t=this._transformManager.getCursor(e);return this.ctx.state.activeTool==="hand"&&t!=="pointer"?"grab":t}_onPointerUp(e){if(this._palmPointers.delete(e.pointerId))return;if(this._pointers.delete(e.pointerId),this._pinching){this._pointers.size<2&&(this._pinching=!1);return}if(!this._ctx.value)return;const t=this._pendingTap;if(t&&t.pointerId===e.pointerId){this._pendingTap=null;const n=this.ctx.state.activeTool,a=Math.hypot(e.clientX-t.down.clientX,e.clientY-t.down.clientY)>10,o=this._getCanvasRect(),h=e.clientX>=o.left&&e.clientX<=o.left+o.width&&e.clientY>=o.top&&e.clientY<=o.top+o.height,c=n==="eyedropper"||n==="stamp"?h?e:null:a?null:t.down;if(c){this._replayingTap=!0;try{this._onPointerDown(c)}finally{this._replayingTap=!1,this._pointers.delete(e.pointerId)}}return}if(this._panning){this._endPan(),this._refreshTransformCursor(e);return}if(this._transformManager){const n=this._getDocPoint(e),a=this._transformManager.onPointerUp(n);a==="commit"||a==="commit-button"?this.commitTransform():a==="cancel-button"&&this.cancelTransform(),(a==="commit-button"||a==="cancel-button")&&(this._floatButtonEnd={time:e.timeStamp,x:e.clientX,y:e.clientY}),this.composite(),this._refreshTransformCursor(e);return}if(this._textSelecting){this._textSelecting=!1;return}const{activeTool:s}=this.ctx.state;if(s==="crop"){this._handleCropPointerUp();return}if(this._drawing&&s!=="pencil"&&s!=="eraser"&&!ce(s)){const n=this._getActiveLayerCtx(),a=n?this._commitStroke(n):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!0,a),this.composite();return}if(this._moveTempCanvas&&s!=="move"){this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(!0),this.composite();return}if(s==="move"&&this._moveTempCanvas){this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite();return}if(s==="select"){this._handleSelectPointerUp(e);return}if(!this._drawing)return;const i=this._getDocPoint(e);let r;if(ce(s)){this._captureBeforeDraw();const n=this._getActiveLayerCtx();n&&(r=Yc(this._startPoint,i,this._brushDescriptor.size),Kr(n,s,this._startPoint,i,this.ctx.state.strokeColor,this.ctx.state.fillColor,this.ctx.state.useFill,this._brushDescriptor.size)),this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}if(s==="pencil"||s==="eraser"){const n=this._getActiveLayerCtx();n&&(this._engine.airbrushActive&&this._engine.tick(e.timeStamp),r=this._commitStroke(n))}this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!1,r),this.composite()}_onPointerLeave(e){if(this._pointerOnCanvas=!1,this._renderPreview(),!this._pinching&&this._pointers.has(e.pointerId)){try{if(this.mainCanvas.hasPointerCapture(e.pointerId))return}catch{}this._onPointerUp(e)}}_onPointerCancel(e){this._palmPointers.delete(e.pointerId)||(this._pointers.delete(e.pointerId),this._pendingTap?.pointerId===e.pointerId&&(this._pendingTap=null),this._pinching?this._pointers.size<2&&(this._pinching=!1):this._cancelCurrentTool(e.pointerId))}_cancelCurrentTool(e,t=!1){try{this.mainCanvas.releasePointerCapture(e)}catch{}if(this._stopAirbrush(),this._engine.cancel(),this._drawing){if(this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._beforeDrawCanvas){const i=this._getActiveLayerCtx();i&&this._restoreBeforeDraw(i),this._beforeDrawCanvas=null}this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite()}if(this._panning&&this._endPan(),this._transformManager?.cancelInteraction(t)&&this.composite(),this._moveTempCanvas){if(this._beforeDrawCanvas){const i=this._getActiveLayerCtx();i&&this._restoreBeforeDraw(i),this._beforeDrawCanvas=null}this._moveTempCanvas=null,this._moveStartPoint=null,this.composite()}this._textSelecting&&(this._textSelecting=!1,t&&this._textAreaEl&&this._textSelectionBeforePress&&([this._textAreaEl.selectionStart,this._textAreaEl.selectionEnd]=this._textSelectionBeforePress,this._renderTextPreview())),this._selectionDrawing&&(this._selectionDrawing=!1,this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh));const s=this._cropDragging||this._cropHandle!==null;if(t&&this._cropDragging?this._cropRect=this._cropRectBeforeNew:t&&this._cropHandle!==null&&this._cropRectOrigin&&(this._cropRect={...this._cropRectOrigin}),this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,s&&this._cropRect){const i=_i(this._ctx.value?.state.cropAspectRatio??"free"),r=this._normalizeCropRect(this._cropRect,i);this._cropRect=r.w<1||r.h<1?null:r,this._cropRect?this._drawCropPreview():this._clearCropPreview()}this._updateCropActions()}_enterPinchMode(e){this._pendingTap=null;for(const[r]of this._pointers)if(r!==e.pointerId){this._cancelCurrentTool(r,!0);break}for(const r of this._pointers.keys())try{this.mainCanvas.setPointerCapture(r)}catch{}this._pinchPair=[...this._pointers.keys()].slice(0,2).join(),this._pinching=!0;const t=[...this._pointers.values()],s=t[1].x-t[0].x,i=t[1].y-t[0].y;this._lastPinchDist=Math.hypot(s,i),this._lastPinchMidX=(t[0].x+t[1].x)/2,this._lastPinchMidY=(t[0].y+t[1].y)/2}_updatePinch(){const e=[...this._pointers.values()];if(e.length<2)return;const t=[...this._pointers.keys()].slice(0,2).join();if(t!==this._pinchPair){this._pinchPair=t,this._lastPinchDist=Math.hypot(e[1].x-e[0].x,e[1].y-e[0].y),this._lastPinchMidX=(e[0].x+e[1].x)/2,this._lastPinchMidY=(e[0].y+e[1].y)/2;return}const s=e[1].x-e[0].x,i=e[1].y-e[0].y,r=Math.hypot(s,i),n=(e[0].x+e[1].x)/2,a=(e[0].y+e[1].y)/2,o=this._clientScale(this._getCanvasRect()),h=(n-this._lastPinchMidX)*o.x,c=(a-this._lastPinchMidY)*o.y;if(this._panX+=h,this._panY+=c,this._lastPinchDist>0){const l=r/this._lastPinchDist,{x:d,y:u}=this._clientToView(n,a),_=(d-this._panX)/this._zoom,f=(u-this._panY)/this._zoom,m=Math.min(q.MAX_ZOOM,Math.max(q.MIN_ZOOM,this._zoom*l));this._panX=d-_*m,this._panY=u-f*m,this._zoom=m}this._lastPinchDist=r,this._lastPinchMidX=n,this._lastPinchMidY=a,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(!1),this._textEditing&&this._renderTextPreview(),this._cropRect&&this._drawCropPreview(),this._dispatchZoomChange(!0)}_handleSelectPointerDown(e){this._transformManager&&this.commitTransform(),this._selectionDrawing=!0,this._startPoint=e}_handleCropPointerDown(e,t=!1){if(this._cropRectBeforeNew=null,this._cropRect){const s=Tn(this._cropRect,e,this._zoom,t?zs.hitRadius:void 0);if(s&&s!=="move"){this._cropHandle=s,this._cropDragOrigin={x:e.x,y:e.y},this._cropRectOrigin={...this._cropRect},this._updateCropActions();return}if(s==="move"){this._cropHandle="move",this._cropDragOrigin={x:e.x,y:e.y},this._cropRectOrigin={...this._cropRect},this._updateCropActions();return}}this._cropRectBeforeNew=this._cropRect,this._cropRect={x:e.x,y:e.y,w:0,h:0},this._cropDragging=!0,this._cropDragOrigin={x:e.x,y:e.y}}_handleSelectPointerMove(e){if(this._selectionDrawing&&this._startPoint){const t=this._getDocPoint(e),s=this.previewCanvas.getContext("2d");s.clearRect(0,0,this._vw,this._vh);const i=Math.min(this._startPoint.x,t.x),r=Math.min(this._startPoint.y,t.y),n=Math.abs(t.x-this._startPoint.x),a=Math.abs(t.y-this._startPoint.y);s.save(),s.translate(this._panX,this._panY),s.scale(this._zoom,this._zoom),ld(s,i,r,n,a,0),s.restore()}}_handleSelectPointerUp(e){if(this._selectionDrawing&&this._startPoint){this._selectionDrawing=!1;const t=this._getDocPoint(e),s=Math.min(this._startPoint.x,t.x),i=Math.min(this._startPoint.y,t.y),r=Math.max(this._startPoint.x,t.x),n=Math.max(this._startPoint.y,t.y);this._startPoint=null;const a=Math.max(0,Math.min(this._docWidth,s)),o=Math.max(0,Math.min(this._docHeight,i)),h=Math.max(0,Math.min(this._docWidth,r)),c=Math.max(0,Math.min(this._docHeight,n)),l=h-a,d=c-o;if(l<2||d<2){this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh);return}const u=Math.round(a),_=Math.round(o),f=Math.round(h)-u,m=Math.round(c)-_;if(f<1||m<1){this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh);return}const p=this._ctx.value?.state;if(!p)return;const g=p.layers.find(y=>y.id===p.activeLayerId);if(g&&f>0&&m>0){const y=g.canvas.getContext("2d");this._captureBeforeDraw();const w=y.getImageData(u,_,f,m);y.clearRect(u,_,f,m),this._transformContentMode="lifted",this._transformManager=new ne(w,{x:u,y:_,w:f,h:m},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),e.pointerType==="touch"&&this._transformManager.setTouchMode(!0),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory(!1)}}}_handleCropPointerMove(e){const t=this._getDocPoint(e),s=_i(this.ctx.state.cropAspectRatio);if(this._cropDragging&&this._cropDragOrigin){let i={x:this._cropDragOrigin.x,y:this._cropDragOrigin.y,w:t.x-this._cropDragOrigin.x,h:t.y-this._cropDragOrigin.y};s&&(i=$n(i,s,"draw")),this._cropRect=i,this._drawCropPreview();return}if(this._cropHandle&&this._cropDragOrigin&&this._cropRectOrigin){const i=t.x-this._cropDragOrigin.x,r=t.y-this._cropDragOrigin.y,n=this._cropRectOrigin;if(this._cropHandle==="move"){let a=n.x+i,o=n.y+r;const h=Math.abs(n.w),c=Math.abs(n.h);a=Math.max(0,Math.min(a,this._docWidth-h)),o=Math.max(0,Math.min(o,this._docHeight-c)),this._cropRect={x:a,y:o,w:h,h:c}}else{let a=this._resizeCropRect(n,this._cropHandle,i,r);s&&(a=$n(a,s,this._cropHandle)),this._cropRect=a}this._drawCropPreview();return}if(this._cropRect){const i=Tn(this._cropRect,t,this._zoom);i&&i!=="move"?this.mainCanvas.style.cursor=this._cropHandleCursor(i):i==="move"?this.mainCanvas.style.cursor="move":this.mainCanvas.style.cursor="crosshair"}}_cropHandleCursor(e){return{nw:"nwse-resize",n:"ns-resize",ne:"nesw-resize",e:"ew-resize",se:"nwse-resize",s:"ns-resize",sw:"nesw-resize",w:"ew-resize"}[e]??"crosshair"}_resizeCropRect(e,t,s,i){let{x:r,y:n,w:a,h:o}=e;switch(t){case"nw":r+=s,n+=i,a-=s,o-=i;break;case"n":n+=i,o-=i;break;case"ne":a+=s,n+=i,o-=i;break;case"e":a+=s;break;case"se":a+=s,o+=i;break;case"s":o+=i;break;case"sw":r+=s,a-=s,o+=i;break;case"w":r+=s,a-=s;break}return{x:r,y:n,w:a,h:o}}_handleCropPointerUp(){const e=_i(this.ctx.state.cropAspectRatio);this._cropDragging&&this._cropRect&&(this._cropRect=this._normalizeCropRect(this._cropRect,e),(this._cropRect.w<1||this._cropRect.h<1)&&(this._cropRect=null)),this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._cropRect&&(this._cropRect=this._normalizeCropRect(this._cropRect,e),(this._cropRect.w<1||this._cropRect.h<1)&&(this._cropRect=null)),this._updateCropActions(),this._cropRect?this._drawCropPreview():this._clearCropPreview()}_normalizeCropRect(e,t){let{x:s,y:i,w:r,h:n}=e;r<0&&(s+=r,r=-r),n<0&&(i+=n,n=-n),t===void 0&&n>0&&r>0&&(t=r/n),s<0&&(r+=s,s=0),i<0&&(n+=i,i=0);const a=this._docWidth-s,o=this._docHeight-i,h=r>a,c=n>o;if(r=Math.min(r,a),n=Math.min(n,o),t&&(h||c)){if(h&&c){const f=r/t,m=n*t;f<=o?n=f:m<=a?r=m:r/n>t?r=n*t:n=r/t}else h?n=r/t:r=n*t;r=Math.min(r,this._docWidth-s),n=Math.min(n,this._docHeight-i)}const l=Math.round(s),d=Math.round(i);let u=Math.round(r),_=Math.round(n);return u=Math.min(u,this._docWidth-l),_=Math.min(_,this._docHeight-d),{x:l,y:d,w:u,h:_}}_drawCropPreview(){if(!this.previewCanvas||!this._cropRect)return;const e=this.previewCanvas.getContext("2d");e.clearRect(0,0,this._vw,this._vh),e.save(),e.translate(this._panX,this._panY),e.scale(this._zoom,this._zoom),dd(e,this._cropRect,this._docWidth,this._docHeight,this._zoom),e.restore()}commitCrop(){if(!this._cropRect)return;const e=Math.max(0,this._cropRect.x),t=Math.max(0,this._cropRect.y),s={x:e,y:t,w:Math.min(this._docWidth,this._cropRect.x+this._cropRect.w)-e,h:Math.min(this._docHeight,this._cropRect.y+this._cropRect.h)-t};if(s.w<1||s.h<1)return;const i=this._ctx.value?.state;if(!i)return;this._transformManager&&this.commitTransform(),this._textEditing&&this._commitText();const r=this._docWidth,n=this._docHeight,a=i.layers.map(h=>{const c=h.canvas.getContext("2d");return{id:h.id,name:h.name,visible:h.visible,opacity:h.opacity,blendMode:h.blendMode,alphaLocked:h.alphaLocked===!0,imageData:c.getImageData(0,0,h.canvas.width,h.canvas.height)}});for(const h of i.layers){const l=h.canvas.getContext("2d").getImageData(s.x,s.y,s.w,s.h),d=document.createElement("canvas");d.width=s.w,d.height=s.h,d.getContext("2d").putImageData(l,0,0),h.canvas=d}const o=i.layers.map(h=>{const c=h.canvas.getContext("2d");return{id:h.id,name:h.name,visible:h.visible,opacity:h.opacity,blendMode:h.blendMode,alphaLocked:h.alphaLocked===!0,imageData:c.getImageData(0,0,h.canvas.width,h.canvas.height)}});this.dispatchEvent(new CustomEvent("crop-commit",{bubbles:!0,composed:!0,detail:{width:s.w,height:s.h}})),this._pushHistoryEntry({type:"crop",beforeLayers:a,afterLayers:o,beforeWidth:r,beforeHeight:n,afterWidth:s.w,afterHeight:s.h}),this._cropRect=null,this._clearCropPreview(),this.composite()}cancelGesture(){for(const e of[...this._pointers.keys()])this._cancelCurrentTool(e,!0);this._pointers.clear(),this._palmPointers.clear(),this._pendingTap=null,this._pinching=!1}cancelCrop(){this._cropRect&&(this._cropRect=null,this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._clearCropPreview())}get hasCropRect(){return this._cropRect!==null}_clearCropPreview(){this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}_createStampAsTransform(e,t,s,i,r=!1){if(e.naturalWidth<=0||e.naturalHeight<=0)return;const n=i/Math.max(e.naturalWidth,e.naturalHeight),a=Math.max(1,Math.round(e.naturalWidth*n)),o=Math.max(1,Math.round(e.naturalHeight*n)),h=Math.round(t-a/2),c=Math.round(s-o/2),l=document.createElement("canvas");l.width=a,l.height=o,l.getContext("2d").drawImage(e,0,0,a,o);const d=l.getContext("2d").getImageData(0,0,a,o);this._transformContentMode="inserted",this._transformManager=new ne(d,{x:h,y:c,w:a,h:o},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this._transformManager.setTouchMode(r),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}hasInteractionInProgress(){return this._cropRect!==null||this._selectionDrawing||!!this._resizeDialog?.asking}dismissResizeDialog(){this._resizeDialog?.dismiss()}_isInert(){for(let e=this;e;e=e.parentNode??e.host??null)if(e instanceof Element&&e.hasAttribute("inert"))return!0;return!1}async _handleExternalImage(e,t){this._transformManager&&this.commitTransform();let s=e.naturalWidth,i=e.naturalHeight;const r=this._docWidth,n=this._docHeight;if(s>r||i>n){const g=this._documentGeneration,y=await this._resizeDialog.show(s,i,r,n);if(g!==this._documentGeneration||!this.isConnected||this._isInert())return;if(y){const w=Math.min(r/s,n/i);s=Math.round(s*w),i=Math.round(i*w)}}this.ctx.addLayer(t),await this.updateComplete,this._captureBeforeDraw(),this._floatIsExternalImage=!0;const a=Math.max(0,-this._panX/this._zoom),o=Math.min(this._docWidth,(this._vw-this._panX)/this._zoom),h=Math.max(0,-this._panY/this._zoom),c=Math.min(this._docHeight,(this._vh-this._panY)/this._zoom),l=o>a?(a+o)/2:this._docWidth/2,d=c>h?(h+c)/2:this._docHeight/2,u=(g,y,w)=>y<=w?Math.min(Math.max(g,0),w-y):g,_=u(Math.round(l-s/2),s,this._docWidth),f=u(Math.round(d-i/2),i,this._docHeight),m=document.createElement("canvas");m.width=s,m.height=i,m.getContext("2d").drawImage(e,0,0,s,i);const p=m.getContext("2d").getImageData(0,0,s,i);this._transformContentMode="inserted",this._transformManager=new ne(p,{x:_,y:f,w:s,h:i},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}getFloatImage(){if(!this._transformManager)return null;const e=this._clippedFloatSnapshot();if(!e)return null;try{return e.canvas.getContext("2d").getImageData(0,0,e.w,e.h)}catch(t){return console.error("Reading the selection failed",t),null}}copySelection(){this._transformManager&&(this._copyFloatToClipboard(),this.commitTransform(),this._notifyHistory())}_copyFloatToClipboard(){const e=this._clippedFloatSnapshot();if(!e)return!1;try{this._clipboard=e.canvas.getContext("2d").getImageData(0,0,e.w,e.h)}catch(t){return console.error("Copy failed",t),!1}return this._clipboardOrigin={x:e.x,y:e.y},this._clipboardRotation=0,this._writeToSystemClipboard(e.canvas),!0}_clippedFloatSnapshot(){try{return this._transformManager.snapshot({x:0,y:0,w:this._docWidth,h:this._docHeight})}catch(e){return console.error("Float snapshot failed",e),null}}_writeToSystemClipboard(e){if(!navigator.clipboard?.write||typeof ClipboardItem>"u")return;const t=new Promise((s,i)=>{e.toBlob(r=>r?s(r):i(new Error("PNG encoding failed")),"image/png")});this._systemClipboardWrite=navigator.clipboard.write([new ClipboardItem({"image/png":t})]).then(()=>!0,()=>!1)}cutSelection(){this._transformManager&&this._copyFloatToClipboard()&&this.deleteSelection()}pasteSelection(){if(!this._clipboard||!this._clipboardOrigin)return;this._cropRect&&this.cancelCrop(),this._transformManager&&this.commitTransform(),this._beforeDrawCanvas||this._captureBeforeDraw();const e=this._clipboard.width,t=this._clipboard.height,s=Math.max(0,Math.min(this._clipboardOrigin.x,this._docWidth-1)),i=Math.max(0,Math.min(this._clipboardOrigin.y,this._docHeight-1)),r=new ImageData(new Uint8ClampedArray(this._clipboard.data),e,t);this._transformContentMode="inserted",this._transformManager=new ne(r,{x:s,y:i,w:e,h:t},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this._clipboardRotation&&(this._transformManager.rotation=this._clipboardRotation*180/Math.PI),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}async paste(){const e=this._systemClipboardWrite;if(e&&!await e){this.isGestureActive()||this.pasteSelection();return}let t=!1;try{const s=await navigator.clipboard.read();t=!0;for(const i of s){const r=i.types.find(h=>h.startsWith("image/"));if(!r)continue;const n=await i.getType(r),a=URL.createObjectURL(n);let o;try{o=await new Promise((h,c)=>{const l=new Image;l.onload=()=>h(l),l.onerror=()=>c(new Error("Image load failed")),l.src=a}),URL.revokeObjectURL(a)}catch{URL.revokeObjectURL(a);continue}if(this._clipboard&&o.naturalWidth===this._clipboard.width&&o.naturalHeight===this._clipboard.height){this.isGestureActive()||this.pasteSelection();return}this.isGestureActive()||await this._handleExternalImage(o,"Pasted Image");return}}catch{}(!t||e)&&!this.isGestureActive()&&this.pasteSelection()}selectAll(){this._transformManager&&this.commitTransform();const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(h=>h.id===e.activeLayerId);if(!t)return;const s=t.canvas.getContext("2d"),i=this._docWidth,r=this._docHeight,n=s.getImageData(0,0,i,r),a=Dn(n);if(!a)return;this._captureBeforeDraw();const o=s.getImageData(a.x,a.y,a.w,a.h);s.clearRect(a.x,a.y,a.w,a.h),this._transformContentMode="lifted",this._transformManager=new ne(o,a,this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory(!1)}selectAllCanvas(){this._transformManager&&this.commitTransform();const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(a=>a.id===e.activeLayerId);if(!t)return;const s=t.canvas.getContext("2d"),i=this._docWidth,r=this._docHeight;this._captureBeforeDraw();const n=s.getImageData(0,0,i,r);s.clearRect(0,0,i,r),this._transformContentMode="lifted",this._transformManager=new ne(n,{x:0,y:0,w:i,h:r},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory(!1)}duplicateInPlace(){if(this._transformManager){const e=this._clippedFloatSnapshot();if(!e)return;let t;try{t=e.canvas.getContext("2d").getImageData(0,0,e.w,e.h)}catch(s){console.error("Duplicate failed",s);return}this._clipboard=t,this._clipboardOrigin={x:e.x,y:e.y},this._clipboardRotation=0,this._writeToSystemClipboard(e.canvas),this.commitTransform(),this._captureBeforeDraw(),this._transformContentMode="inserted",this._transformManager=new ne(t,{x:e.x,y:e.y,w:e.w,h:e.h},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}else this.pasteSelection()}deleteSelection(){if(this._transformManager){if(this._floatIsExternalImage){this.cancelExternalFloat();return}if(this._transformContentMode==="inserted"&&!this._beforeDrawCanvas){this.cancelTransform();return}this._beforeDrawCanvas||this._captureBeforeDraw(),this._transformManager.dispose(),this._transformManager=null,this._floatCommitCache=null,this._transformContentMode="lifted",this._pushDrawHistory(!0),this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}}clearSelection({keepCrop:e=!1}={}){if(this._textEditing&&this._commitText(),this._cropRect&&!e&&this.cancelCrop(),this._drawing){const t=this._getActiveLayerCtx(),s=t?this._commitStroke(t):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!1,s),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite()}this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._selectionDrawing&&(this._selectionDrawing=!1,this._startPoint=null,this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)),this._transformManager&&this.commitTransform()}cancelExternalFloat(){if(!this._floatIsExternalImage||!this._transformManager)return;const e=this.ctx.state.activeLayerId;if(this._transformManager.dispose(),this._transformManager=null,this._floatCommitCache=null,this._floatIsExternalImage=!1,this._transformContentMode="lifted",this._selectionDrawing=!1,this._beforeDrawCanvas=null,this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this._dispatchTransformChange(),this._historyIndex>=0){let t=-1;for(let s=this._historyIndex;s>=0;s--){const i=this._history[s];if(i.type==="add-layer"&&i.layer.id===e){t=s;break}}if(t>=0){const s=this._history.slice(0,t),r=this._history.slice(t,this._historyIndex+1).filter(n=>{const a=this._getEntryLayerId(n);return a===null||a!==e});this._history=[...s,...r],this._historyIndex=this._history.length-1}}this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:e}})),this.composite(),this._notifyHistory()}get hasExternalFloat(){return this._floatIsExternalImage&&this._transformManager!==null}getFloatKey(){const e=this._ctx.value?.state.activeLayerId;return!this._transformManager||!e?null:{layerId:e,key:this._transformManager.getStateKey()}}connectedCallback(){super.connectedCallback();const e=document.createElement("textarea");e.style.cssText="position:fixed;top:0;left:0;width:1px;height:1px;opacity:0;border:0;padding:0;margin:0;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;",e.setAttribute("autocomplete","off"),e.setAttribute("autocorrect","off"),e.setAttribute("autocapitalize","off"),e.setAttribute("spellcheck","false"),e.setAttribute("aria-label","Text"),e.tabIndex=-1,e.addEventListener("input",()=>{this._textEditing&&(this._startTextCursorBlink(),this._renderTextPreview(),this._dispatchPendingTextChange())}),e.addEventListener("keydown",t=>this._onTextKeydown(t)),e.addEventListener("selectionchange",()=>{this._textEditing&&(this._startTextCursorBlink(),this._renderTextPreview())}),this._textAreaEl=e,this.addEventListener("wheel",this._onWheel,{passive:!1}),this.addEventListener("dragover",this._onDragOver),this.addEventListener("dragenter",this._onDragEnter),this.addEventListener("dragleave",this._onDragLeave),this.addEventListener("drop",this._onDrop),window.addEventListener("blur",this._onWindowBlur),document.addEventListener("copy",this._onOtherCopy,!0),document.addEventListener("cut",this._onOtherCopy,!0),window.addEventListener("resize",this._invalidateCanvasRect),window.addEventListener("scroll",this._invalidateCanvasRect,!0),this.hasUpdated&&(this.shadowRoot.appendChild(e),this._observeSize(),this.scheduleComposite(!1),this._cropRect&&this._drawCropPreview())}disconnectedCallback(){super.disconnectedCallback(),this._textCursorInterval&&(clearInterval(this._textCursorInterval),this._textCursorInterval=0),this._textAreaEl&&(this._textAreaEl.remove(),this._textAreaEl=null),this._resizeObserver?.disconnect(),this._resizeObserver=null,this._unwatchDevicePixelRatio(),this._stopAirbrush(),this._compositeScheduler.cancel(),this._viewportChangeScheduler.cancel(),this._viewportChangePending=!1,this._transformManager&&(this._transformManager.dispose(),this._transformManager=null,this._transformContentMode="lifted",this._floatIsExternalImage=!1),this._transformViewCanvas&&(this._transformViewCanvas.width=this._transformViewCanvas.height=0,this._transformViewCanvas=null),this._pointers.clear(),this._pinching=!1,this._panning=!1,this._panPointerId=-1,this.removeEventListener("wheel",this._onWheel),this.removeEventListener("dragover",this._onDragOver),this.removeEventListener("dragenter",this._onDragEnter),this.removeEventListener("dragleave",this._onDragLeave),this.removeEventListener("drop",this._onDrop),window.removeEventListener("blur",this._onWindowBlur),document.removeEventListener("copy",this._onOtherCopy,!0),document.removeEventListener("cut",this._onOtherCopy,!0),window.removeEventListener("resize",this._invalidateCanvasRect),window.removeEventListener("scroll",this._invalidateCanvasRect,!0)}_renderTextPreview(){if(!this._textEditing||!this._textAreaEl||!this.previewCanvas)return;const e=this.previewCanvas.getContext("2d");e.clearRect(0,0,this._vw,this._vh);const t=this.ctx.state,s=this._textAreaEl.value,{fontFamily:i,fontSize:r,fontBold:n,fontItalic:a,strokeColor:o}=t;e.save(),e.translate(this._panX,this._panY),e.scale(this._zoom,this._zoom),In(e,s,this._textPosition.x,this._textPosition.y,r,i,n,a,o);const h=En(e,s,r,i,n,a),c=r*Ls,l=this._textAreaEl.selectionStart??0,d=this._textAreaEl.selectionEnd??l,u=s.split(`
`);e.font=js(r,i,n,a),e.textBaseline="top";const _=w=>{let x=0;for(let S=0;S<u.length;S++){if(x+u[S].length>=w){const E=w-x,D=u[S].substring(0,E);return{line:S,x:this._textPosition.x+e.measureText(D).width,y:this._textPosition.y+S*c}}x+=u[S].length+1}const b=u.length-1;return{line:b,x:this._textPosition.x+e.measureText(u[b]).width,y:this._textPosition.y+b*c}};if(l!==d){e.fillStyle="rgba(99, 102, 241, 0.3)";const w=_(l),x=_(d);if(w.line===x.line)e.fillRect(w.x,w.y,x.x-w.x,c);else{const b=e.measureText(u[w.line]).width;e.fillRect(w.x,w.y,this._textPosition.x+b-w.x,c);for(let S=w.line+1;S<x.line;S++){const E=e.measureText(u[S]).width;e.fillRect(this._textPosition.x,this._textPosition.y+S*c,E,c)}e.fillRect(this._textPosition.x,x.y,x.x-this._textPosition.x,c)}}else if(this._textCursorVisible){const w=_(l);e.fillStyle=o,e.fillRect(w.x,w.y,2/this._zoom,r)}const f=4/this._zoom,m=this._textPosition.x-f,p=this._textPosition.y-f,g=Math.max(h.width,r)+f*2,y=h.height+f*2;e.strokeStyle="rgba(99, 102, 241, 0.6)",e.lineWidth=1/this._zoom,e.setLineDash([4/this._zoom,4/this._zoom]),e.strokeRect(m,p,g,y),e.restore()}_getTextBoundingBox(){const e=this.ctx.state,t=this._textAreaEl?.value??"",s=this.previewCanvas.getContext("2d"),i=En(s,t,e.fontSize,e.fontFamily,e.fontBold,e.fontItalic),r=4/this._zoom;return{x:this._textPosition.x-r,y:this._textPosition.y-r,w:Math.max(i.width,e.fontSize)+r*2,h:i.height+r*2}}_pointToTextOffset(e){if(!this._textAreaEl)return 0;const t=this.ctx.state,i=this._textAreaEl.value.split(`
`),{fontSize:r,fontFamily:n,fontBold:a,fontItalic:o}=t,h=r*Ls,c=this.previewCanvas.getContext("2d");c.save(),c.font=js(r,n,a,o),c.textBaseline="top";const l=e.y-this._textPosition.y;let d=Math.floor(l/h);d=Math.max(0,Math.min(d,i.length-1));const u=i[d],_=e.x-this._textPosition.x;let f=0;for(let p=0;p<=u.length;p++){const g=c.measureText(u.substring(0,p)).width;if(p>0){const w=(c.measureText(u.substring(0,p-1)).width+g)/2;_>=w&&(f=p)}else _<0&&(f=0)}c.restore();let m=0;for(let p=0;p<d;p++)m+=i[p].length+1;return m+f}_startTextCursorBlink(){this._textCursorVisible=!0,this._textCursorInterval&&clearInterval(this._textCursorInterval),this._textCursorInterval=window.setInterval(()=>{this._textCursorVisible=!this._textCursorVisible,this._renderTextPreview()},530)}_stopTextCursorBlink(){this._textCursorInterval&&(clearInterval(this._textCursorInterval),this._textCursorInterval=0),this._textCursorVisible=!1}_onTextKeydown(e){this._textEditing&&(e.key==="Escape"?(e.preventDefault(),e.stopPropagation(),this._textAreaEl&&this._textAreaEl.value.length>0?this._commitText():this._cancelText()):e.key==="Tab"&&e.preventDefault())}_commitText(){if(!this._textEditing||!this._textAreaEl)return;const e=this._textAreaEl.value;if(!e){this._cancelText();return}const t=this.ctx.state;this._captureBeforeDraw();const s=this._getActiveLayerCtx();s?(In(s,e,this._textPosition.x,this._textPosition.y,t.fontSize,t.fontFamily,t.fontBold,t.fontItalic,t.strokeColor),this._pushDrawHistory(),this.composite()):this._beforeDrawCanvas=null,this._endTextEditing()}_cancelText(){this._endTextEditing()}_endTextEditing(){this._textEditing=!1,this._stopTextCursorBlink(),this._textAreaEl&&(this._textAreaEl.value="",this.shadowRoot?.activeElement===this._textAreaEl&&ee(this),this._textAreaEl.blur()),this._dispatchPendingTextChange(),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}render(){return v`
      <canvas
        id="main"
        @pointerenter=${this._onPointerEnter}
        @pointerdown=${this._onPointerDown}
        @pointermove=${this._onPointerMove}
        @pointerup=${this._onPointerUp}
        @pointerleave=${this._onPointerLeave}
        @pointercancel=${this._onPointerCancel}
        @contextmenu=${this._onContextMenu}
      ></canvas>
      <canvas
        id="preview"
        style="position:absolute;top:0;left:0;pointer-events:none;"
      ></canvas>
      ${this._cropActionsVisible?v`
        <div class="crop-actions" role="group" aria-label="Crop actions">
          <button class="apply" type="button" title="Apply crop (Enter)" @click=${this._onApplyCropClick}>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
            Apply
          </button>
          <button type="button" title="Cancel crop (Esc)" @click=${this._onCancelCropClick}>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>
            Cancel
          </button>
        </div>
      `:""}
      <resize-dialog></resize-dialog>
    `}};q.styles=le`
    :host {
      display: block;
      flex: 1;
      overflow: hidden;
      position: relative;
      background: #3a3a3a;
    }

    canvas {
      display: block;
      touch-action: none;
    }

    #main {
      background: transparent;
      cursor: crosshair;
    }

    :host(.drop-target) #main {
      outline: 3px dashed #4a90d9;
      outline-offset: -3px;
    }

    /* Crop has no keyboard on touch devices, so the commit/cancel actions
       live on the canvas instead of behind Enter/Escape. */
    .crop-actions {
      position: absolute;
      left: 50%;
      bottom: 1rem;
      transform: translateX(-50%);
      display: flex;
      gap: 0.5rem;
      padding: 0.375rem;
      border-radius: 0.625rem;
      background: rgba(30, 30, 30, 0.92);
      border: 1px solid #555;
      box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
    }

    .crop-actions button {
      display: inline-flex;
      align-items: center;
      gap: 0.375rem;
      min-height: 44px;
      padding: 0 0.875rem;
      border: 1px solid #555;
      border-radius: 0.5rem;
      background: #3a3a3a;
      color: #ddd;
      font-family: inherit;
      font-size: 0.875rem;
      cursor: pointer;
      touch-action: manipulation;
    }

    .crop-actions button.apply {
      background: #2f7d32;
      border-color: #3f9e43;
      color: #fff;
    }
  `;q.MIN_ZOOM=.1;q.MAX_ZOOM=10;q.ZOOM_STEP=1.1;os([as("#main")],q.prototype,"mainCanvas",2);os([as("#preview")],q.prototype,"previewCanvas",2);os([as("resize-dialog")],q.prototype,"_resizeDialog",2);os([T()],q.prototype,"_cropActionsVisible",2);q=os([de("drawing-canvas")],q);var Kd=Object.defineProperty,Zd=Object.getOwnPropertyDescriptor,se=(e,t,s,i)=>{for(var r=i>1?void 0:i?Zd(t,s):t,n=e.length-1,a;n>=0;n--)(a=e[n])&&(r=(i?a(t,s,r):a(r))||r);return i&&r&&Kd(t,s,r),r};let Nt=class extends Bt{constructor(){super(...arguments),this._ctx=new Me(this,{context:Fe,subscribe:!0}),this._thumbnailScheduler=As(()=>this._updateThumbnails(),250),this._onComposited=e=>{e.detail?.contentChanged!==!1&&this._thumbnailScheduler.schedule()},this._onDocClick=()=>{this._closeContextMenu(),this._dropdownOpen=!1},this._onDocKeyDown=e=>{e.key!=="Escape"||this._editingLayerId||!Ds(this,e)||(this._contextMenuOpen||this._dropdownOpen?(e.stopPropagation(),e.preventDefault(),this._closeContextMenu(),this._dropdownOpen=!1):this._sheetOpen&&(e.stopPropagation(),e.preventDefault(),this.closeSheet()))},this._sheetOpen=!1,this._sheetY=0,this._sheetDragging=!1,this._syncingSheet=!1,this._sheetDragStartY=0,this._sheetDragStartTranslate=0,this._sheetSnapHalf=0,this._sheetSnapFull=0,this._sheetDragTimestamps=[],this._contextMenuOpen=!1,this._contextMenuX=0,this._contextMenuY=0,this._dropdownOpen=!1,this._editingLayerId=null,this._draggedLayerId=null,this._dragPointerId=null,this._dragStartY=0,this._dragCurrentY=0,this._dragThreshold=5,this._dragActivated=!1,this._opacityBefore=null,this._eyeOpen=v`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>`,this._eyeClosed=v`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/><line x1="1" y1="1" x2="23" y2="23"/></svg>`,this._plusIcon=v`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>`,this._trashIcon=v`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>`,this._lockOpen=v`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.5-2"/></svg>`,this._lockClosed=v`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></svg>`,this._focusAfterReorder=null,this._dragTarget=null,this._onResize=()=>{this._sheetOpen&&!this._sheetDragging&&this._recalcSnapPoints()},this._lastThumbLayers=null,this._lastThumbRowCount=-1,this._miniCheckerboardPattern=null}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback(),this.getRootNode().addEventListener("composited",this._onComposited),window.addEventListener("resize",this._onResize),document.addEventListener("click",this._onDocClick),document.addEventListener("keydown",this._onDocKeyDown,!0)}disconnectedCallback(){super.disconnectedCallback(),this.getRootNode().removeEventListener("composited",this._onComposited),this._thumbnailScheduler.cancel(),window.removeEventListener("resize",this._onResize),document.removeEventListener("click",this._onDocClick),document.removeEventListener("keydown",this._onDocKeyDown,!0)}_toggleAlphaLock(e,t){t.stopPropagation(),this.ctx.setLayerAlphaLocked(e.id,!e.alphaLocked)}_toggleVisibility(e,t){t.stopPropagation(),this.ctx.setLayerVisibility(e.id,!e.visible)}_startRename(e,t){t.stopPropagation(),this._editingLayerId=e}_commitRename(e,t){if(this._editingLayerId!==e)return;const s=t.value.trim();s&&s!==this._getLayerById(e)?.name&&this.ctx.renameLayer(e,s),this._editingLayerId=null}_onRenameKeyDown(e,t){t.stopPropagation(),t.key==="Enter"?(this._commitRename(e,t.target),ee(this)):t.key==="Escape"&&(t.preventDefault(),this._editingLayerId=null,ee(this))}_onRenameBlur(e,t){this._commitRename(e,t.target)}_moveUp(e,t){t.stopPropagation();const s=this.ctx.state.layers,i=s.findIndex(r=>r.id===e.id);i<s.length-1&&this._reorderFrom(t,e.id,i+1)}_moveDown(e,t){t.stopPropagation();const i=this.ctx.state.layers.findIndex(r=>r.id===e.id);i>0&&this._reorderFrom(t,e.id,i-1)}_reorderFrom(e,t,s){const i=e.currentTarget;i&&this.shadowRoot?.activeElement===i&&(this._focusAfterReorder={layerId:t,toIndex:s,title:i.title}),this.ctx.reorderLayer(t,s)}_restoreReorderFocus(){const e=this._focusAfterReorder;if(!e||this.ctx.state.layers[e.toIndex]?.id!==e.layerId)return;this._focusAfterReorder=null,this.shadowRoot?.querySelector(`[data-layer-id="${e.layerId}"]`)?.querySelector(`.reorder-btn[title="${e.title}"]`)?.focus()}_onReorderPointerDown(e,t){t.button===0&&(t.composedPath()[0].closest?.("input, select, button, textarea")||(this._draggedLayerId=e.id,this._dragPointerId=t.pointerId,this._dragStartY=t.clientY,this._dragCurrentY=t.clientY,this._dragActivated=!1,this._dragTarget=t.currentTarget))}_onReorderPointerMove(e){if(this._dragPointerId!==e.pointerId||!this._draggedLayerId)return;if(this._dragCurrentY=e.clientY,!this._dragActivated){if(Math.abs(this._dragCurrentY-this._dragStartY)<this._dragThreshold)return;this._dragActivated=!0,this._dragTarget&&this._dragTarget.setPointerCapture(e.pointerId)}this._clearDropIndicators();const t=this.shadowRoot?.querySelectorAll(".layer-row");if(t)for(const s of t){const i=s.getBoundingClientRect();if(this._dragCurrentY>=i.top&&this._dragCurrentY<=i.bottom){const r=i.top+i.height/2;this._dragCurrentY<r?s.classList.add("drop-above"):s.classList.add("drop-below");break}}}_onReorderPointerUp(e){if(this._dragPointerId!==e.pointerId)return;const t=this._draggedLayerId;if(!t||!this._dragActivated){this._clearDragState();return}const s=this.shadowRoot?.querySelectorAll(".layer-row");if(!s){this._clearDragState();return}let i=null,r=!1;for(const n of s){const a=n.getBoundingClientRect();if(this._dragCurrentY>=a.top&&this._dragCurrentY<=a.bottom){i=n.dataset.layerId??null;const o=a.top+a.height/2;r=this._dragCurrentY<o;break}}if(i&&i!==t){const n=this.ctx.state.layers,a=n.findIndex(o=>o.id===i);if(a!==-1){let o=r?a+1:a;const h=n.findIndex(c=>c.id===t);h<o&&(o-=1),o=Math.max(0,Math.min(n.length-1,o)),h!==o&&this.ctx.reorderLayer(t,o)}}this._clearDragState()}_onReorderPointerCancel(e){this._clearDragState()}_clearDropIndicators(){this.shadowRoot?.querySelectorAll(".layer-row")?.forEach(t=>t.classList.remove("drop-above","drop-below"))}_clearDragState(){this._draggedLayerId=null,this._dragPointerId=null,this._dragActivated=!1,this._dragTarget=null,this._clearDropIndicators()}openSheet(){this._sheetSnapFull=0,this._sheetY=0,this._sheetOpen=!0,this.updateComplete.then(()=>this._measureSnaps())}_measureSnaps(){const e=this.shadowRoot?.querySelector(".sheet"),t=e?e.offsetHeight:window.innerHeight*.9;this._sheetSnapHalf=Math.floor(t*.5)}closeSheet(){this._sheetOpen=!1,this._sheetY=0,!this._syncingSheet&&this.ctx?.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}_recalcSnapPoints(){const e=this.shadowRoot?.querySelector(".sheet"),t=e?e.offsetHeight:window.innerHeight*.9,s=this._sheetSnapHalf;if(this._sheetSnapFull=0,this._sheetSnapHalf=Math.floor(t*.5),s>0){if(this._sheetY===s)this._sheetY=this._sheetSnapHalf;else if(this._sheetY!==this._sheetSnapFull){const i=Math.abs(this._sheetY-this._sheetSnapHalf),r=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=i<r?this._sheetSnapHalf:this._sheetSnapFull}}}_onSheetHandlePointerDown(e){this._sheetDragging=!0,this._sheetDragStartY=e.clientY,this._sheetDragStartTranslate=this._sheetY,this._sheetDragTimestamps=[{y:e.clientY,t:Date.now()}],e.target.setPointerCapture(e.pointerId)}_onSheetHandlePointerMove(e){if(!this._sheetDragging)return;const t=e.clientY-this._sheetDragStartY,s=Math.max(this._sheetSnapFull,this._sheetDragStartTranslate+t);this._sheetY=s,this._sheetDragTimestamps.push({y:e.clientY,t:Date.now()}),this._sheetDragTimestamps.length>5&&this._sheetDragTimestamps.shift()}_onSheetHandlePointerUp(e){if(!this._sheetDragging)return;this._sheetDragging=!1;const t=this._sheetDragTimestamps;let s=0;if(t.length>=2){const a=t[t.length-1],o=t[t.length-2],h=a.t-o.t;h>0&&(s=(a.y-o.y)/h)}const i=window.innerHeight*.75;if(this._sheetY>i||s>.5){this.closeSheet();return}const r=Math.abs(this._sheetY-this._sheetSnapHalf),n=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=r<n?this._sheetSnapHalf:this._sheetSnapFull}_onSheetHandlePointerCancel(e){if(!this._sheetDragging)return;this._sheetDragging=!1;const t=Math.abs(this._sheetY-this._sheetSnapHalf),s=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=t<s?this._sheetSnapHalf:this._sheetSnapFull}_onOpacityPointerDown(e){this._opacityBefore=e.opacity}_onOpacityInput(e,t){if(this._opacityBefore===null){const i=this._getLayerById(e);i&&(this._opacityBefore=i.opacity)}const s=Number(t.target.value)/100;this.ctx.setLayerOpacity(e,s)}_onOpacityChange(e,t){const s=Number(t.target.value)/100,i=this._opacityBefore;this._opacityBefore=null,i!==null&&i!==s&&this.dispatchEvent(new CustomEvent("commit-opacity",{bubbles:!0,composed:!0,detail:{layerId:e,before:i,after:s}}))}_onContextMenu(e,t){e.preventDefault(),this._selectLayer(t),this._contextMenuX=e.clientX,this._contextMenuY=e.clientY,this._contextMenuOpen=!0}_closeContextMenu(){this._contextMenuOpen=!1}_fitContextMenu(){const e=this.shadowRoot?.querySelector(".context-menu");if(!e)return;const t=e.getBoundingClientRect(),s=t.left-this._contextMenuX,i=t.top-this._contextMenuY,r=Math.max(0,Math.min(this._contextMenuX,window.innerWidth-t.width)),n=Math.max(0,Math.min(this._contextMenuY,window.innerHeight-t.height));(s||i||r!==this._contextMenuX||n!==this._contextMenuY)&&(e.style.left=`${r-s}px`,e.style.top=`${n-i}px`)}_getLayerById(e){return this.ctx.state.layers.find(t=>t.id===e)}_selectLayer(e){this.ctx.setActiveLayer(e)}render(){if(!this._ctx.value)return v``;if(this.ctx.isMobile)return this._renderMobileSheet();const{layersPanelOpen:e}=this.ctx.state;return e?v`
      <div class="panel">
        ${this._renderLayersList()}
      </div>
    `:v`
        <div class="panel collapsed">
          <div class="collapsed-strip">
            <button
              class="expand-btn"
              title="Show layers"
              @click=${()=>this.ctx.toggleLayersPanel()}
            >&#9654;</button>
            <span class="vertical-label">Layers</span>
          </div>
        </div>
      `}_renderMobileSheet(){const e=this._sheetOpen?`transform: translateY(${this._sheetY}px);${this._sheetDragging?"transition:none;":""}`:"transform: translateY(100%);";return v`
      <div
        class="sheet-backdrop ${this._sheetOpen?"open":""}"
        @click=${()=>this.closeSheet()}
      ></div>
      <div
        class="sheet ${this._sheetOpen?"open":""}"
        style=${e}
      >
        <div
          class="sheet-handle"
          @pointerdown=${t=>this._onSheetHandlePointerDown(t)}
          @pointermove=${t=>this._onSheetHandlePointerMove(t)}
          @pointerup=${t=>this._onSheetHandlePointerUp(t)}
          @pointercancel=${t=>this._onSheetHandlePointerCancel(t)}
        >
          <div class="sheet-handle-bar"></div>
        </div>
        <div class="sheet-content">
          ${this._renderLayersList()}
        </div>
      </div>
    `}_renderLayersList(){const{layers:e,activeLayerId:t}=this.ctx.state,s=[...e].reverse();return v`
      <div class="header">
        <span class="header-title">Layers</span>
        <button
          class="collapse-btn"
          title="Hide layers"
          @click=${()=>this.ctx.toggleLayersPanel()}
        >&#9664; hide</button>
      </div>

      <div class="layer-list">
        ${$a(s,i=>i.id,i=>this._renderLayerRow(i,e,t))}
      </div>

      ${this._contextMenuOpen?v`
        <div
          class="context-menu"
          style="left:${this._contextMenuX}px;top:${this._contextMenuY}px"
          @click=${i=>i.stopPropagation()}
        >
          <button
            class="context-menu-item"
            ?disabled=${e.findIndex(i=>i.id===t)===0}
            @click=${()=>{this.ctx.mergeLayerDown(t),this._closeContextMenu()}}
          >Merge Down</button>
          <button
            class="context-menu-item"
            ?disabled=${e.filter(i=>i.visible).length<2}
            @click=${()=>{this.ctx.mergeVisibleLayers(),this._closeContextMenu()}}
          >Merge Visible</button>
          <button
            class="context-menu-item"
            ?disabled=${e.length<=1}
            @click=${()=>{this.ctx.flattenImage(),this._closeContextMenu()}}
          >Flatten Image</button>
        </div>
      `:A}

      <div class="action-bar-wrapper">
        ${this._dropdownOpen?v`
          <div class="dropdown-menu" @click=${i=>i.stopPropagation()}>
            <button
              ?disabled=${e.findIndex(i=>i.id===t)===0}
              @click=${()=>{this.ctx.mergeLayerDown(t),this._dropdownOpen=!1}}
            >Merge Down</button>
            <button
              ?disabled=${e.filter(i=>i.visible).length<2}
              @click=${()=>{this.ctx.mergeVisibleLayers(),this._dropdownOpen=!1}}
            >Merge Visible</button>
            <button
              ?disabled=${e.length<=1}
              @click=${()=>{this.ctx.flattenImage(),this._dropdownOpen=!1}}
            >Flatten Image</button>
          </div>
        `:A}
        <div class="action-bar">
          <button
            class="action-btn"
            title="Add layer"
            @click=${()=>this.ctx.addLayer()}
          >${this._plusIcon} Add</button>
          <button
            class="action-btn"
            title="Delete layer"
            ?disabled=${e.length<=1}
            @click=${()=>this.ctx.deleteLayer(t)}
          >${this._trashIcon} Delete</button>
          <button
            class="action-btn"
            title="More actions"
            @click=${i=>{i.stopPropagation(),this._dropdownOpen=!this._dropdownOpen}}
          >&#8943;</button>
        </div>
      </div>
    `}_renderLayerRow(e,t,s){const i=e.id===s,r=t.findIndex(h=>h.id===e.id),n=r===t.length-1,a=r===0,o=this._editingLayerId===e.id;return v`
      <div
        class="layer-row ${i?"active":""} ${this._draggedLayerId===e.id?"dragging":""}"
        data-layer-id=${e.id}
        @click=${()=>this._selectLayer(e.id)}
        @contextmenu=${h=>this._onContextMenu(h,e.id)}
        @pointerdown=${h=>this._onReorderPointerDown(e,h)}
        @pointermove=${h=>this._onReorderPointerMove(h)}
        @pointerup=${h=>this._onReorderPointerUp(h)}
        @pointercancel=${h=>this._onReorderPointerCancel(h)}
      >
        <div
          class="layer-row-main"
        >
          <button
            class="vis-btn ${e.visible?"":"hidden"}"
            title=${e.visible?"Hide layer":"Show layer"}
            @click=${h=>this._toggleVisibility(e,h)}
          >
            ${e.visible?this._eyeOpen:this._eyeClosed}
          </button>

          <canvas class="layer-thumb" width="48" height="36"></canvas>

          ${o?v`<input
                class="layer-name-input"
                aria-label="Layer name"
                .value=${e.name}
                @keydown=${h=>this._onRenameKeyDown(e.id,h)}
                @blur=${h=>this._onRenameBlur(e.id,h)}
                @click=${h=>h.stopPropagation()}
                ${this._autoFocusDirective()}
              />`:v`<span
                class="layer-name"
                @dblclick=${h=>this._startRename(e.id,h)}
              >${e.name}</span>`}

          ${this.ctx.isMobile?v`
            <button
              class="rename-btn"
              title="Rename"
              @click=${h=>{h.stopPropagation(),this._startRename(e.id,h)}}
            >
              <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M11.5 1.5l3 3L5 14H2v-3L11.5 1.5z"/>
              </svg>
            </button>
          `:""}

          <button
            class="lock-btn"
            aria-label=${`Lock transparent pixels of ${e.name}`}
            aria-pressed=${e.alphaLocked?"true":"false"}
            title=${e.alphaLocked?"Unlock transparent pixels":"Lock transparent pixels"}
            @click=${h=>this._toggleAlphaLock(e,h)}
          >
            ${e.alphaLocked?this._lockClosed:this._lockOpen}
          </button>

          <div class="reorder-btns">
            <button
              class="reorder-btn"
              title="Move up"
              aria-disabled=${n?"true":"false"}
              @click=${h=>this._moveUp(e,h)}
            >&#9650;</button>
            <button
              class="reorder-btn"
              title="Move down"
              aria-disabled=${a?"true":"false"}
              @click=${h=>this._moveDown(e,h)}
            >&#9660;</button>
          </div>
        </div>

        ${i?v`
            <div class="opacity-row">
              <select class="blend-mode-select"
                aria-label=${`Blend mode of ${e.name}`}
                .value=${e.blendMode}
                @change=${h=>this.ctx.setLayerBlendMode(e.id,h.target.value)}>
                ${Object.entries(To).map(([h,c])=>v`
                  <option value=${h}>${c}</option>
                `)}
              </select>
            </div>
            <div class="opacity-row">
              <input
                type="range"
                min="0"
                max="100"
                aria-label=${`Opacity of ${e.name}`}
                .value=${String(Math.round(e.opacity*100))}
                @pointerdown=${()=>this._onOpacityPointerDown(e)}
                @input=${h=>this._onOpacityInput(e.id,h)}
                @change=${h=>this._onOpacityChange(e.id,h)}
              />
              <span class="opacity-value">${Math.round(e.opacity*100)}%</span>
            </div>
          `:A}
      </div>
    `}_autoFocusDirective(){return requestAnimationFrame(()=>{const e=this.shadowRoot?.querySelector(".layer-name-input");e&&(e.focus(),e.select())}),A}willUpdate(){this.ctx?.isMobile&&!this._syncingSheet&&(this._syncingSheet=!0,this.ctx.state.layersPanelOpen&&!this._sheetOpen?this.openSheet():!this.ctx.state.layersPanelOpen&&this._sheetOpen&&this.closeSheet(),this._syncingSheet=!1)}updated(e){super.updated(e),this._contextMenuOpen&&["_contextMenuOpen","_contextMenuX","_contextMenuY"].some(i=>e.has(i))&&this._fitContextMenu(),this._restoreReorderFocus();const t=this._ctx.value?.state.layers??null,s=this.shadowRoot?.querySelectorAll(".layer-thumb").length??0;(t!==this._lastThumbLayers||s!==this._lastThumbRowCount)&&(this._lastThumbLayers=t,this._lastThumbRowCount=s,this._updateThumbnails())}_updateThumbnails(){const e=this._ctx.value?.state.layers??[],t=this.shadowRoot?.querySelectorAll(".layer-thumb");if(!t)return;const s=[...e].reverse();t.forEach((i,r)=>{const n=s[r];if(!n)return;const a=i.getContext("2d");a.clearRect(0,0,i.width,i.height),this._drawMiniCheckerboard(a,i.width,i.height),a.globalAlpha=n.opacity,a.drawImage(n.canvas,0,0,i.width,i.height),a.globalAlpha=1})}_drawMiniCheckerboard(e,t,s){if(!this._miniCheckerboardPattern){const r=document.createElement("canvas");r.width=8,r.height=8;const n=r.getContext("2d");n.fillStyle="#ffffff",n.fillRect(0,0,8,8),n.fillStyle="#e0e0e0",n.fillRect(4,0,4,4),n.fillRect(0,4,4,4),this._miniCheckerboardPattern=e.createPattern(r,"repeat")}e.fillStyle=this._miniCheckerboardPattern,e.fillRect(0,0,t,s)}};Nt.styles=le`
    :host {
      display: flex;
      flex-direction: column;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 0.8125rem;
      color: #ddd;
      user-select: none;
    }

    /* ── Panel (expanded) ─────────────────────── */
    .panel {
      display: flex;
      flex-direction: column;
      flex: 1;
      overflow: hidden;
    }

    .panel.collapsed {
    }

    /* ── Collapsed strip ──────────────────────── */
    .collapsed-strip {
      display: flex;
      flex-direction: column;
      align-items: center;
      width: 32px;
      height: 100%;
      padding-top: 8px;
      gap: 4px;
    }

    .expand-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      font-size: 0.75rem;
      padding: 0;
    }

    .expand-btn:hover {
      background: #444;
      color: #fff;
    }

    .vertical-label {
      writing-mode: vertical-rl;
      text-orientation: mixed;
      color: #888;
      font-size: 0.6875rem;
      letter-spacing: 0.05em;
      margin-top: 6px;
    }

    /* ── Header ───────────────────────────────── */
    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 6px 8px;
      border-bottom: 1px solid #444;
      flex-shrink: 0;
    }

    .header-title {
      font-weight: 600;
      font-size: 0.8125rem;
    }

    .collapse-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 2px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #aaa;
      cursor: pointer;
      font-size: 0.6875rem;
      padding: 2px 6px;
      white-space: nowrap;
    }

    .collapse-btn:hover {
      background: #444;
      color: #fff;
    }

    /* ── Layer list ────────────────────────────── */
    .layer-list {
      flex: 1;
      overflow-y: auto;
      overflow-x: hidden;
      scrollbar-width: thin;
      scrollbar-color: #555 transparent;
    }

    /* ── Layer row ─────────────────────────────── */
    .layer-row {
      position: relative;
      display: flex;
      flex-direction: column;
      padding: 6px 8px;
      min-height: 44px;
      background: #3a3a3a;
      border-bottom: 1px solid #333;
      cursor: pointer;
      transition: background 0.1s ease;
      touch-action: none;
    }

    .layer-row.dragging {
      opacity: 0.4;
    }

    .layer-row.drop-above::before {
      content: '';
      position: absolute;
      top: -1px;
      left: 0;
      right: 0;
      height: 2px;
      background: #5b8cf7;
    }

    .layer-row.drop-below::after {
      content: '';
      position: absolute;
      bottom: -1px;
      left: 0;
      right: 0;
      height: 2px;
      background: #5b8cf7;
    }

    .layer-row:hover {
      background: #424242;
    }

    .layer-row.active {
      background: #3a3a5c;
    }

    .layer-row-main {
      display: flex;
      align-items: center;
      gap: 4px;
      min-height: 24px;
      cursor: grab;
    }

    .layer-row-main:active {
      cursor: grabbing;
    }

    /* ── Visibility button ─────────────────────── */
    .vis-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border: none;
      border-radius: 4px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .vis-btn:hover {
      background: #555;
      color: #fff;
    }

    .vis-btn.hidden {
      color: #666;
    }

    .vis-btn svg {
      width: 14px;
      height: 14px;
    }

    /* ── Lock transparency button ──────────────── */
    .lock-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border: none;
      border-radius: 4px;
      background: transparent;
      color: #666;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .lock-btn:hover {
      background: #555;
      color: #fff;
    }

    .lock-btn[aria-pressed='true'] {
      color: #f2c14e;
    }

    .lock-btn svg {
      width: 14px;
      height: 14px;
    }

    /* ── Layer thumbnail ──────────────────────── */
    .layer-thumb {
      width: 48px;
      height: 36px;
      border-radius: 3px;
      border: 1px solid #555;
      flex-shrink: 0;
    }

    /* ── Layer name ─────────────────────────────── */
    .layer-name {
      flex: 1;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: 0.75rem;
      line-height: 22px;
    }

    .layer-name-input {
      flex: 1;
      min-width: 0;
      font-size: 0.75rem;
      background: #222;
      border: 1px solid #5b8cf7;
      border-radius: 3px;
      color: #ddd;
      padding: 1px 4px;
      outline: 2px solid transparent;
      font-family: inherit;
    }

    /* ── Reorder buttons ───────────────────────── */
    /* Every button in the row is at least 24px square (WCAG 2.5.8 target
       size); no other spacing keeps two stacked arrows clear of each other. */
    .reorder-btns {
      display: flex;
      flex-direction: column;
      gap: 0px;
      flex-shrink: 0;
    }

    .reorder-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border: none;
      border-radius: 3px;
      background: transparent;
      color: #888;
      cursor: pointer;
      font-size: 0.5rem;
      padding: 0;
      line-height: 1;
    }

    .reorder-btn:hover:not([aria-disabled="true"]) {
      background: #555;
      color: #fff;
    }

    /* Not :disabled: a button at the end of the list keeps keyboard focus
       (it would jump to the other one, and a repeated key move it back). */
    .reorder-btn[aria-disabled="true"] {
      opacity: 0.25;
      cursor: default;
    }

    /* ── Opacity slider ────────────────────────── */
    .opacity-row {
      display: flex;
      align-items: center;
      gap: 6px;
      margin-top: 4px;
      padding-left: 26px;
    }

    .opacity-row input[type="range"] {
      flex: 1;
      height: 4px;
      accent-color: #5b8cf7;
      min-width: 0;
    }

    .opacity-value {
      font-size: 0.6875rem;
      color: #aaa;
      min-width: 28px;
      text-align: right;
    }

    /* ── Action bar ────────────────────────────── */
    .action-bar {
      display: flex;
      justify-content: center;
      gap: 6px;
      padding: 6px 8px;
      border-top: 1px solid #444;
      flex-shrink: 0;
    }

    .action-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 4px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      font-size: 0.6875rem;
      padding: 4px 10px;
      font-family: inherit;
      transition: all 0.15s ease;
    }

    .action-btn:hover:not(:disabled) {
      background: #444;
      color: #fff;
    }

    .action-btn:disabled {
      opacity: 0.3;
      cursor: default;
    }

    .action-btn svg {
      width: 14px;
      height: 14px;
    }

    /* ── Context menu ──────────────────────────── */
    .context-menu {
      position: fixed;
      z-index: 300;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 6px;
      padding: 4px 0;
      min-width: 160px;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
    }

    .context-menu-item {
      display: block;
      width: 100%;
      padding: 6px 14px;
      border: none;
      background: transparent;
      color: #ddd;
      font-size: 0.8125rem;
      font-family: inherit;
      text-align: left;
      cursor: pointer;
    }

    .context-menu-item:hover:not(:disabled) {
      background: #5b8cf7;
      color: #fff;
    }

    .context-menu-item:disabled {
      color: #666;
      cursor: default;
    }

    /* ── Dropdown menu ─────────────────────────── */
    .action-bar-wrapper {
      position: relative;
    }

    .dropdown-menu {
      position: absolute;
      bottom: 100%;
      right: 0;
      margin-bottom: 4px;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 6px;
      padding: 4px 0;
      min-width: 160px;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
      z-index: 300;
    }

    .dropdown-menu button {
      display: block;
      width: 100%;
      padding: 6px 14px;
      border: none;
      background: transparent;
      color: #ddd;
      font-size: 0.8125rem;
      font-family: inherit;
      text-align: left;
      cursor: pointer;
    }

    .dropdown-menu button:hover:not(:disabled) {
      background: #5b8cf7;
      color: #fff;
    }

    .dropdown-menu button:disabled {
      color: #666;
      cursor: default;
    }

    /* ── Mobile bottom sheet ───────────────────── */
    .sheet-backdrop {
      display: none;
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.4);
      z-index: 200;
    }

    .sheet-backdrop.open {
      display: block;
    }

    .sheet {
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      max-height: 90vh;
      background: #2c2c2c;
      border-radius: 16px 16px 0 0;
      z-index: 201;
      display: flex;
      flex-direction: column;
      transition: transform 0.3s ease;
      transform: translateY(100%);
      will-change: transform;
      padding-bottom: env(safe-area-inset-bottom);
    }

    .sheet-handle {
      display: flex;
      justify-content: center;
      padding: 8px 0;
      cursor: grab;
      touch-action: none;
    }

    .sheet-handle-bar {
      width: 36px;
      height: 4px;
      border-radius: 2px;
      background: #666;
    }

    .sheet-content {
      flex: 1;
      overflow-y: auto;
      touch-action: pan-y;
      -webkit-overflow-scrolling: touch;
    }

    .rename-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 28px;
      height: 28px;
      border: none;
      border-radius: 4px;
      background: transparent;
      color: #888;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .rename-btn:hover {
      background: #444;
      color: #ddd;
    }

    .blend-mode-select {
      width: 100%;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 4px;
      padding: 2px 4px;
      font-size: 0.75rem;
      cursor: pointer;
    }
  `;se([T()],Nt.prototype,"_sheetOpen",2);se([T()],Nt.prototype,"_sheetY",2);se([T()],Nt.prototype,"_contextMenuOpen",2);se([T()],Nt.prototype,"_contextMenuX",2);se([T()],Nt.prototype,"_contextMenuY",2);se([T()],Nt.prototype,"_dropdownOpen",2);se([T()],Nt.prototype,"_editingLayerId",2);se([T()],Nt.prototype,"_draggedLayerId",2);Nt=se([de("layers-panel")],Nt);var Qd=Object.defineProperty,Jd=Object.getOwnPropertyDescriptor,Ks=(e,t,s,i)=>{for(var r=i>1?void 0:i?Jd(t,s):t,n=e.length-1,a;n>=0;n--)(a=e[n])&&(r=(i?a(t,s,r):a(r))||r);return i&&r&&Qd(t,s,r),r};let ft=class extends Bt{constructor(){super(...arguments),this._ctx=new Me(this,{context:Fe,subscribe:!0}),this._minimapCanvas=document.createElement("canvas"),this._isFullscreen=!1,this._onFullscreenChange=()=>{this._isFullscreen=!!document.fullscreenElement},this._toggleFullscreen=()=>{document.fullscreenElement?document.exitFullscreen():document.documentElement.requestFullscreen()},this._minimapScheduler=As(()=>this._renderMinimap(),100),this._onComposited=e=>{e.detail?.contentChanged!==!1&&(this._docImageDirty=!0),this._minimapScheduler.schedule()},this._minimapScale=1,this._minimapOffsetX=0,this._minimapOffsetY=0,this._docImage=null,this._docImageDirty=!0,this._docImageLayers=null,this._docImageDocW=0,this._docImageDocH=0,this._dragging=!1,this._dragOffsetX=0,this._dragOffsetY=0,this._onMinimapPointerDown=e=>{if(e.button!==0)return;const t=this._getMinimapPoint(e);if(this._isInsideViewportRect(t.x,t.y)){const s=this._getViewportRectInMinimap();this._dragging=!0,this._dragOffsetX=t.x-s.x,this._dragOffsetY=t.y-s.y,this._minimapCanvas.setPointerCapture(e.pointerId)}else{this._panToMinimapPoint(t.x,t.y);const s=this._getViewportRectInMinimap();this._dragging=!0,this._dragOffsetX=s.w/2,this._dragOffsetY=s.h/2,this._minimapCanvas.setPointerCapture(e.pointerId)}},this._onMinimapPointerMove=e=>{if(!this._dragging)return;const t=this._getMinimapPoint(e),s=t.x-this._dragOffsetX,i=t.y-this._dragOffsetY,{zoom:r}=this.ctx,n=this._minimapScale,a=(s-this._minimapOffsetX)/n,o=(i-this._minimapOffsetY)/n,h=-a*r,c=-o*r;this.dispatchEvent(new CustomEvent("navigator-pan",{bubbles:!0,composed:!0,detail:{panX:h,panY:c}}))},this._onMinimapPointerUp=e=>{this._dragging&&(this._dragging=!1,this._minimapCanvas.releasePointerCapture(e.pointerId))},this._onSliderInput=e=>{const t=parseInt(e.target.value,10);this._dispatchZoom(this._sliderToZoom(t))},this._onZoomIn=()=>{this._dispatchZoom(this.ctx.zoom*ft.ZOOM_STEP)},this._onZoomOut=()=>{this._dispatchZoom(this.ctx.zoom/ft.ZOOM_STEP)},this._editingZoom=!1,this._zoomInputValue="",this._onZoomInputFocus=e=>{this._editingZoom=!0,this._zoomInputValue=Math.round(this.ctx.zoom*100).toString();const t=e.target;requestAnimationFrame(()=>{this._editingZoom&&this.shadowRoot?.activeElement===t&&t.select()})},this._onZoomInputBlur=()=>{this._editingZoom&&this._commitZoomInput(),this._editingZoom=!1},this._onZoomInputKeydown=e=>{if(e.key==="Enter"||e.key==="Escape"){e.preventDefault(),e.key==="Enter"&&this._commitZoomInput(),this._editingZoom=!1;const t=e.target;ee(this),this.shadowRoot?.activeElement===t&&t.blur()}e.stopPropagation()},this._onZoomInputChange=e=>{this._zoomInputValue=e.target.value}}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback(),this.getRootNode().addEventListener("composited",this._onComposited),document.addEventListener("fullscreenchange",this._onFullscreenChange)}disconnectedCallback(){super.disconnectedCallback(),this.getRootNode().removeEventListener("composited",this._onComposited),this._minimapScheduler.cancel(),document.removeEventListener("fullscreenchange",this._onFullscreenChange)}_renderMinimap(){if(!this._ctx.value)return;const{state:e}=this.ctx,{layers:t,documentWidth:s,documentHeight:i}=e,r=this._minimapCanvas,n=this.shadowRoot?.querySelector(".minimap-container");if(!n)return;const a=n.clientWidth-12;if(a<=0)return;const o=150,h=s/i;let c=a,l=c/h;l>o&&(l=o,c=l*h);const d=window.devicePixelRatio||1,u=Math.round(c*d),_=Math.round(l*d);(r.width!==u||r.height!==_)&&(r.width=u,r.height=_),r.style.width=`${c}px`,r.style.height=`${l}px`;const f=r.getContext("2d");f.setTransform(d,0,0,d,0,0);const m=Math.min(c/s,l/i);this._minimapScale=m;const p=s*m,g=i*m;this._minimapOffsetX=(c-p)/2,this._minimapOffsetY=(l-g)/2,f.fillStyle="#3a3a3a",f.fillRect(0,0,c,l);const y=Math.max(1,Math.round(p*d)),w=Math.max(1,Math.round(g*d));let x=this._docImage;if((!x||x.width!==y||x.height!==w)&&(x=this._docImage=document.createElement("canvas"),x.width=y,x.height=w,this._docImageDirty=!0),this._docImageDirty||this._docImageLayers!==t||this._docImageDocW!==s||this._docImageDocH!==i){this._docImageDirty=!1,this._docImageLayers=t,this._docImageDocW=s,this._docImageDocH=i;const b=x.getContext("2d");b.fillStyle="#ffffff",b.fillRect(0,0,y,w);const S=t.some(E=>E.visible&&E.blendMode!=="normal");for(const E of t)E.visible&&(b.globalAlpha=E.opacity,S&&(b.globalCompositeOperation=Qe(E.blendMode)),b.drawImage(E.canvas,0,0,y,w));b.globalAlpha=1,b.globalCompositeOperation="source-over"}f.drawImage(x,this._minimapOffsetX,this._minimapOffsetY,p,g),this._drawViewportRect(f,m,c,l)}_drawViewportRect(e,t,s,i){const{zoom:r,panX:n,panY:a,viewportWidth:o,viewportHeight:h}=this.ctx,c=this._minimapOffsetX+-n/r*t,l=this._minimapOffsetY+-a/r*t,d=o/r*t,u=h/r*t,_=Math.max(0,c),f=Math.max(0,l),m=Math.min(s-_,d-(_-c)),p=Math.min(i-f,u-(f-l));m<=0||p<=0||(e.fillStyle="rgba(255, 68, 68, 0.1)",e.fillRect(_,f,m,p),e.strokeStyle="#ff4444",e.lineWidth=1.5,e.strokeRect(_,f,m,p))}_getMinimapPoint(e){const t=this._minimapCanvas.getBoundingClientRect();return{x:e.clientX-t.left,y:e.clientY-t.top}}_getViewportRectInMinimap(){const{zoom:e,panX:t,panY:s,viewportWidth:i,viewportHeight:r}=this.ctx,n=this._minimapScale;return{x:this._minimapOffsetX+-t/e*n,y:this._minimapOffsetY+-s/e*n,w:i/e*n,h:r/e*n}}_isInsideViewportRect(e,t){const s=this._getViewportRectInMinimap();return e>=s.x&&e<=s.x+s.w&&t>=s.y&&t<=s.y+s.h}_panToMinimapPoint(e,t){const{zoom:s,viewportWidth:i,viewportHeight:r}=this.ctx,n=this._minimapScale,a=(e-this._minimapOffsetX)/n,o=(t-this._minimapOffsetY)/n,h=i/2-a*s,c=r/2-o*s;this.dispatchEvent(new CustomEvent("navigator-pan",{bubbles:!0,composed:!0,detail:{panX:h,panY:c}}))}_zoomToSlider(e){const{MIN_ZOOM:t,MAX_ZOOM:s,SLIDER_MAX:i}=ft,r=Math.log(e/t)/Math.log(s/t);return Math.round(r*i)}_sliderToZoom(e){const{MIN_ZOOM:t,MAX_ZOOM:s,SLIDER_MAX:i}=ft,r=e/i;return t*Math.pow(s/t,r)}_dispatchZoom(e){const t=Math.min(ft.MAX_ZOOM,Math.max(ft.MIN_ZOOM,e));this.dispatchEvent(new CustomEvent("navigator-zoom",{bubbles:!0,composed:!0,detail:{zoom:t}}))}_commitZoomInput(){const e=this._zoomInputValue.replace("%","").trim(),t=parseFloat(e);if(isNaN(t)||t<=0)return;const s=t/100;this._dispatchZoom(s)}render(){if(!this._ctx.value)return v``;if(!this.ctx.state.layersPanelOpen)return v``;if(this.ctx.isMobile)return v``;const e=this.ctx.zoom,t=this._zoomToSlider(e),s=Math.round(e*100),i=this._editingZoom?this._zoomInputValue:`${s}%`;return v`
      <div class="section">
        <div class="header">
          <span class="header-title">Navigator</span>
        </div>
        <div class="minimap-container"
          @pointerdown=${this._onMinimapPointerDown}
          @pointermove=${this._onMinimapPointerMove}
          @pointerup=${this._onMinimapPointerUp}
        >
          ${this._minimapCanvas}
        </div>
        <div class="zoom-controls">
          <button class="zoom-btn" title="Zoom out" @click=${this._onZoomOut}>&minus;</button>
          <input
            type="range"
            class="zoom-slider"
            aria-label="Zoom"
            min="0"
            max="${ft.SLIDER_MAX}"
            step="1"
            .value=${String(t)}
            @input=${this._onSliderInput}
          />
          <button class="zoom-btn" title="Zoom in" @click=${this._onZoomIn}>+</button>
          <input
            type="text"
            class="zoom-input"
            aria-label="Zoom percentage"
            .value=${i}
            @focus=${this._onZoomInputFocus}
            @blur=${this._onZoomInputBlur}
            @keydown=${this._onZoomInputKeydown}
            @input=${this._onZoomInputChange}
          />
          <button
            class="fullscreen-btn"
            title=${this._isFullscreen?"Exit fullscreen":"Fullscreen"}
            @click=${this._toggleFullscreen}
          >
            ${this._isFullscreen?v`<svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M5 2v3H2M14 5h-3V2M11 14v-3h3M2 11h3v3"/></svg>`:v`<svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4"/></svg>`}
          </button>
        </div>
      </div>
    `}};ft.MIN_ZOOM=.1;ft.MAX_ZOOM=10;ft.ZOOM_STEP=1.1;ft.SLIDER_MAX=1e3;ft.styles=le`
    :host {
      display: block;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 0.8125rem;
      color: #ddd;
      user-select: none;
    }

    .section {
      border-bottom: 1px solid #444;
    }

    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 6px 8px;
      border-bottom: 1px solid #444;
    }

    .header-title {
      font-weight: 500;
      font-size: 0.75rem;
      color: #ccc;
    }

    .minimap-container {
      padding: 6px;
    }

    canvas {
      display: block;
      width: 100%;
      border-radius: 3px;
      background: #3a3a3a;
      cursor: crosshair;
    }

    .zoom-controls {
      display: flex;
      align-items: center;
      gap: 4px;
      padding: 4px 6px 6px;
    }

    .zoom-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      border: 1px solid #555;
      border-radius: 4px;
      background: #444;
      color: #ccc;
      cursor: pointer;
      font-size: 14px;
      padding: 0;
      line-height: 1;
      flex-shrink: 0;
    }

    .zoom-btn:hover {
      background: #555;
      color: #fff;
    }

    .zoom-slider {
      flex: 1;
      height: 4px;
      -webkit-appearance: none;
      appearance: none;
      background: #555;
      border-radius: 2px;
      outline: 2px solid transparent;
      min-width: 0;
    }

    .zoom-slider::-webkit-slider-thumb {
      -webkit-appearance: none;
      width: 12px;
      height: 12px;
      border-radius: 50%;
      background: #bbb;
      border: 1px solid #888;
      cursor: grab;
    }

    .zoom-slider::-webkit-slider-thumb:active {
      cursor: grabbing;
    }

    .zoom-input {
      width: 38px;
      background: #333;
      border: 1px solid #555;
      border-radius: 3px;
      padding: 1px 3px;
      text-align: center;
      font-size: 0.6875rem;
      color: #ddd;
      font-family: inherit;
      flex-shrink: 0;
    }

    .zoom-input:focus {
      outline: 1px solid #007bff;
      border-color: #007bff;
    }

    .fullscreen-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      border: 1px solid #555;
      border-radius: 4px;
      background: #444;
      color: #ccc;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .fullscreen-btn:hover {
      background: #555;
      color: #fff;
    }
  `;Ks([T()],ft.prototype,"_isFullscreen",2);Ks([T()],ft.prototype,"_editingZoom",2);Ks([T()],ft.prototype,"_zoomInputValue",2);ft=Ks([de("navigator-panel")],ft);var tu=Object.defineProperty,eu=Object.getOwnPropertyDescriptor,qa=(e,t,s,i)=>{for(var r=i>1?void 0:i?eu(t,s):t,n=e.length-1,a;n>=0;n--)(a=e[n])&&(r=(i?a(t,s,r):a(r))||r);return i&&r&&tu(t,s,r),r};let Hs=class extends Bt{constructor(){super(...arguments),this._options=null,this._resolve=null,this._returnFocus=null}show(e){return this._answer(!1),this._returnFocus=bi(),this._options=e,new Promise(t=>{this._resolve=t,this.updateComplete.then(()=>{const s=this._dialog;if(!(!s||this._resolve!==t)){if(!this.isConnected){this._answer(!1);return}s.open||s.showModal(),s.querySelector("button.cancel")?.focus()}})})}get open(){return this._resolve!==null}dismiss(){this._answer(!1)}get _dialog(){return this.renderRoot.querySelector("dialog")}_answer(e){const t=this._resolve;if(!t)return;this._resolve=null;const s=this._dialog,i=!!s&&s.contains(bi());if(s?.open&&s.close(),i){const r=this._returnFocus;r?.isConnected&&r.focus({preventScroll:!0}),(!r?.isConnected||bi()!==r)&&ee(this)}this._returnFocus=null,t(e)}disconnectedCallback(){super.disconnectedCallback(),this._answer(!1)}render(){const e=this._options;return v`
      <dialog
        aria-labelledby="message"
        @cancel=${t=>{t.preventDefault(),this._answer(!1)}}
        @close=${t=>{t.target.open||this._answer(!1)}}
        @keydown=${t=>t.stopPropagation()}
      >
        <p id="message">${e?.message??""}</p>
        <div class="buttons">
          <button class="cancel" @click=${()=>this._answer(!1)}>${e?.cancelLabel??"Cancel"}</button>
          <button class=${e?.danger?"danger":"primary"} @click=${()=>this._answer(!0)}>${e?.confirmLabel??"OK"}</button>
        </div>
      </dialog>
    `}};Hs.styles=le`
    dialog {
      box-sizing: border-box;
      background: #2a2a2a;
      color: #e0e0e0;
      border: 1px solid #555;
      border-radius: 8px;
      padding: 20px 24px;
      max-width: min(400px, calc(100vw - 32px));
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 14px;
    }
    dialog::backdrop {
      background: rgba(0, 0, 0, 0.5);
    }
    p {
      margin: 0 0 16px;
      line-height: 1.5;
    }
    .buttons {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      justify-content: flex-end;
    }
    button {
      padding: 8px 16px;
      border-radius: 4px;
      border: 1px solid #555;
      background: #3a3a3a;
      color: #e0e0e0;
      cursor: pointer;
      font-size: 13px;
    }
    button:hover {
      background: #4a4a4a;
    }
    button:focus-visible {
      outline: 2px solid #8bbcf0;
      outline-offset: 2px;
    }
    button.primary {
      background: #4a90d9;
      border-color: #4a90d9;
      color: #fff;
    }
    button.primary:hover {
      background: #5aa0e9;
    }
    button.danger {
      background: #c0392b;
      border-color: #c0392b;
      color: #fff;
    }
    button.danger:hover {
      background: #d64535;
    }
  `;qa([T()],Hs.prototype,"_options",2);Hs=qa([de("confirm-dialog")],Hs);function bi(){let e=document.activeElement;for(;e?.shadowRoot?.activeElement;)e=e.shadowRoot.activeElement;return e instanceof HTMLElement?e:null}var su=Object.defineProperty,iu=Object.getOwnPropertyDescriptor,z=(e,t,s,i)=>{for(var r=i>1?void 0:i?iu(t,s):t,n=e.length-1,a;n>=0;n--)(a=e[n])&&(r=(i?a(t,s,r):a(r))||r);return i&&r&&su(t,s,r),r};const ru=768,nu=800,wi=16384;function Nn(e,t){if(!(e>0&&t>0&&e<=wi&&t<=wi))throw new RangeError(`Document size ${e}×${t} is outside 1–${wi} pixels`)}function Xn(e){return e==="select"||e==="hand"}function au(e,t){return t?e<=nu:e<ru}function ou(e){const t=new Set;try{xe(e.entry,t)}catch{const s=(i,r)=>{if(!(!i||typeof i!="object"||r>4))for(const[n,a]of Object.entries(i))n==="blobRef"&&typeof a=="string"?t.add(a):s(a,r+1)};s(e.entry,0)}return[...t].filter(s=>typeof s=="string"&&s!=="")}const hu=512*1024*1024;let $=class extends Bt{constructor(){super(),this._layerCounter=0,this._canUndo=!1,this._canRedo=!1,this._saving=!1,this._viewportZoom=1,this._viewportPanX=0,this._viewportPanY=0,this._viewportWidth=800,this._viewportHeight=600,this._currentProject=null,this._projectList=[],this._isMobile=!1,this._mobileObserver=null,this.embedded=!1,this._storageState="loading",this._storageBlocked=!1,this._updateRequired=!1,this._storageClosed=!1,this._updateReady=!1,this._updateNoticeDismissed=!1,this._reload=()=>location.reload(),this._ownsBackend=!1,this._autosave=!0,this._ready=new Promise((t,s)=>{this._resolveReady=t,this._rejectReady=s}),this._savedDocument={top:null,trimmed:0,generation:0},this._documentGeneration=0,this._exportedDocument=null,this._exportMarks=new WeakMap,this._lastReportedModified=!1,this._documentReplacement=Promise.resolve(),this._dirty=!1,this._wakeSaveSleep=null,this._saveTimer=null,this._saveInProgress=!1,this._savePromise=null,this._saveRequested=!1,this._forceFlushNextSave=!1,this._dirtyVersion=0,this._contentVersion=0,this._savedContentVersion=-1,this._storedContentVersion=0,this._storedHistoryTop=null,this._savedThumbKey=null,this._unsavedWork=!1,this._savedHistory=new Map,this._nextHistoryRecordIndex=0,this._historyNeedsRewrite=!1,this._savedLayerBlobs=new Map,this._trackedProjectId=null,this._trackingGeneration=0,this._projectLoads=0,this._desktopLayersPanelOpen=null,this._onBeforeUnload=t=>{if(!this.embedded){if(this._stranded){t.preventDefault();return}if(this._storageClosed){this._hasUnsavedWork()&&t.preventDefault();return}if(this._ownsProject(this._currentProject?.id??"")&&(this.canvas?.clearSelection({keepCrop:!0}),this.canvas?.flushViewportChange?.(),this._dirty)){const s=this._hasUnsavedWork();this._flushPendingSave(),s&&t.preventDefault()}}},this._onPageHide=()=>{this._ownsProject(this._currentProject?.id??"")&&(this.canvas?.clearSelection({keepCrop:!0}),this.canvas?.flushViewportChange?.(),this._dirty&&this._flushPendingSave())},this._onVisibilityChange=()=>{!document.hidden&&this._readOnly&&this._editHere(!1),document.hidden&&(this.canvas?.clearSelection({keepCrop:!0}),this.canvas?.flushViewportChange?.(),this._dirty&&this._flushPendingSave(),(this._updateReady||this._storageClosed)&&this._reloadWhileHidden())},this._onOnline=()=>this._retrySaveNow(),this._brushOps=new Set,this._readOnly=!1,this._waitingForTab=!1,this._projectLock=null,this._contentLock=null,this._lockRequest=0,this._lockWait=null,this._lockAsked=Promise.resolve(),this._tabs=null,this._handOverAgain=!1,this._handOverAgainAsks=[],this._handOverAsks=[],this._keptAsks=new Set,this._handingOver=!1,this._keptElsewhere=!1,this._otherTabSilent=!1,this._forceTakeOver=null,this._claimEnd=null,this._claimCancelled=!1,this._stealing=!1,this._noCanvas=!1,this._overlayProjects=!1,this._stranded=!1,this._projectGone=!1,this._notice="",this._keeping=!1,this._opening=!1,this._switching=!1,this._switchRequest=0,this._deletingProject=null,this._replacing=!1,this._enterGeneration=0,this._enterAbort=null,this._claiming=!1,this._saveFailed=!1,this._saveError=!1,this._lastSaveError=null,this._saveRetryTimer=null,this._saveRetryDelay=$.SAVE_RETRY_MIN,this._keepError="",this._lastSaveAnnounce=0,this._onKeyDown=t=>{const s=(t.ctrlKey||t.metaKey)&&!t.shiftKey&&!t.altKey&&(t.key.toLowerCase()==="s"||t.code==="KeyS")&&this.embedded;if(s&&t.preventDefault(),this._readOnly||this._switching||this._replacing)return;if(s){t.repeat||this._requestSave();return}if(this._isTextEntryTarget(t))return;const i=t.composedPath()[0],r=i instanceof HTMLInputElement||i instanceof HTMLSelectElement,n=t.key.length===1||["Backspace","Delete","Dead","Process"].includes(t.key),a=["Enter","ArrowLeft","ArrowRight","ArrowUp","ArrowDown","Home","End"].includes(t.key);if(!t.ctrlKey&&!t.metaKey&&!t.altKey&&(n||a&&!r)&&this.canvas?.focusText?.())return;if(t.key==="Escape"&&this.canvas?.hasExternalFloat){t.preventDefault(),this.canvas.cancelExternalFloat();return}if(t.key==="Escape"&&this.canvas?.isTransformActive()){t.preventDefault(),this.canvas.cancelTransform();return}if(t.key==="Enter"&&this.canvas?.isTransformActive()){t.preventDefault(),this.canvas.commitTransform();return}const o=t.ctrlKey||t.metaKey,h=t.key.toLowerCase();if(this.canvas?.isGestureActive?.()&&(o&&["t","c","x","v","d","a"].includes(h)||t.key==="Delete"||t.key==="Backspace")){t.preventDefault();return}if(o&&h==="t"){t.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.isTransformActive()||this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty("setting")),this.canvas?.enterTransformMode();return}if(o&&h==="z"&&!t.shiftKey)t.preventDefault(),this.canvas?.undo();else if(o&&(h==="y"||h==="z"&&t.shiftKey))t.preventDefault(),this.canvas?.redo();else if(o&&h==="c")t.preventDefault(),this.canvas?.copySelection();else if(o&&h==="x")t.preventDefault(),this.canvas?.cutSelection();else if(o&&h==="v")t.preventDefault(),this.canvas?.paste();else if(o&&h==="d")t.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.isTransformActive()||this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty("setting")),this.canvas?.duplicateInPlace();else if((t.key==="Delete"||t.key==="Backspace")&&(this._state.activeTool==="select"||this._state.activeTool==="stamp"||this.canvas?.isTransformActive()))t.preventDefault(),this.canvas?.deleteSelection();else if(t.key==="Enter"&&this._state.activeTool==="crop"&&this.canvas?.hasCropRect)t.preventDefault(),this.canvas.commitCrop();else if(t.key==="Escape")this._state.activeTool==="crop"&&this.canvas?.hasCropRect?(t.preventDefault(),this.canvas.cancelCrop()):this.canvas?.hasExternalFloat?(t.preventDefault(),this.canvas.cancelExternalFloat()):(this.canvas?.isTextEditing?.()&&t.preventDefault(),this.canvas?.clearSelection());else if(o&&h==="a"&&t.shiftKey)t.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty("setting")),this.canvas?.selectAllCanvas();else if(o&&h==="a"&&!t.shiftKey)t.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty("setting")),this.canvas?.selectAll();else if(t.key==="0"&&o)t.preventDefault(),this.canvas?.zoomToFit();else if(o&&(t.key==="="||t.key==="+"))t.preventDefault(),this.canvas?.zoomIn();else if(o&&t.key==="-")t.preventDefault(),this.canvas?.zoomOut();else if(!o&&!t.altKey&&(h==="["||h==="]")){if(t.preventDefault(),this._state.activeTool==="stamp"){const u=this._state.stampSize,_=h==="]"?Math.max(u+1,Math.round(u*1.1)):Math.min(u-1,Math.round(u/1.1));this._updateStampSize(_);return}const c=this._state.brush.size,l=150,d=1;if(h==="]"){const u=Math.min(l,Math.max(c+1,Math.round(c*1.1)));this._updateBrush({size:u})}else{const u=Math.max(d,Math.min(c-1,Math.round(c/1.1)));this._updateBrush({size:u})}}else if(!o&&!t.altKey&&(t.key==="{"||t.key==="}")){t.preventDefault();const c=this._state.brush.hardness;t.key==="}"?this._updateBrush({hardness:Math.round(Math.min(1,c+.1)*10)/10}):this._updateBrush({hardness:Math.round(Math.max(0,c-.1)*10)/10})}else if(!o&&!t.altKey&&!t.shiftKey&&h.length===1){const c=zc(h);c&&c!==this._state.activeTool&&(t.preventDefault(),this.canvas?.cancelCrop(),Xn(c)&&this.canvas?.isTransformActive()||this.canvas?.clearSelection(),this._state={...this._state,activeTool:c},this._markDirty("setting"))}},this._unsavableProjectId=null,this._layoutWidth=0,this._strayKeysOurs=!1,this._onDocumentFocusIn=t=>{this._strayKeysOurs=t.composedPath().includes(this)},this._onDocumentPointerDown=t=>{this._strayKeysOurs=t.composedPath().includes(this)},this._onStrayKeyDown=t=>{!this._strayKeysOurs||t.defaultPrevented||t.key==="Tab"||this.getRootNode().activeElement===this||t.composedPath().includes(this)||!(t.target instanceof Node)||!Gl(t.target,this)||(this.focus({preventScroll:!0}),this.getRootNode().activeElement===this&&this._onKeyDown(t))},this._detached=!1,this._backendClosed=!1,this._backendReopen=null,this._ignoreDrop=t=>t.preventDefault(),this._ready.catch(()=>{});const e=this._createLayer(800,600);this._state={activeTool:"pencil",strokeColor:"#000000",fillColor:"#ff0000",useFill:!1,brush:Is(),activePreset:"round",isPresetModified:!1,stampImage:null,activeStampId:null,stampSize:Ei,layers:[e],activeLayerId:e.id,layersPanelOpen:!0,documentWidth:800,documentHeight:600,cropAspectRatio:"free",fontFamily:"sans-serif",fontSize:24,fontBold:!1,fontItalic:!1,eyedropperSampleAll:!0,stabilizer:0,childMode:!1},this._provider=new Qs(this,{context:Fe,initialValue:this._buildContextValue()})}_createLayer(e,t){this._layerCounter++;const s=document.createElement("canvas");return s.width=e,s.height=t,{id:jt(),name:`Layer ${this._layerCounter}`,visible:!0,opacity:1,blendMode:"normal",alphaLocked:!1,canvas:s}}_confirm(e){return this._confirmDialog?.show(e)??Promise.resolve(!1)}_dismissDialogs(){this.canvas?.dismissResizeDialog(),this._confirmDialog?.dismiss()}_snapshotLayer(e){const t=e.canvas.getContext("2d");return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode,alphaLocked:e.alphaLocked===!0,imageData:t.getImageData(0,0,e.canvas.width,e.canvas.height)}}_snapshotAllLayers(){return this._state.layers.map(e=>this._snapshotLayer(e))}_compositeLayers(e,t="#ffffff"){const s=this._state.documentWidth,i=this._state.documentHeight,r=document.createElement("canvas");r.width=s,r.height=i;const n=r.getContext("2d");t&&(n.fillStyle=t,n.fillRect(0,0,s,i));for(const a of e)n.globalAlpha=a.opacity,n.globalCompositeOperation=Qe(a.blendMode),n.drawImage(a.canvas,0,0),n.globalCompositeOperation="source-over";return n.globalAlpha=1,r}_flushPendingSave(){this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),this._save(!0)}async _flushWhileDetached(){const e=this._currentProject?.id;await this._flushPendingSaveAndWait();let t=$.SAVE_RETRY_MIN/2;for(let s=0;s<4;s++){if(this.isConnected||!this._saveError||!this._dirty||!e||!this._canSaveProject(e)||(await new Promise(i=>setTimeout(i,t)),t*=2,this.isConnected))return;await this._flushPendingSaveAndWait()}}async _flushPendingSaveAndWait(){this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),await this._save(!0)}_markDirty(e="work"){e==="work"&&this._contentVersion++,this._autosave&&(e==="work"&&(this._unsavedWork=!0),this._dirty=!0,this._dirtyVersion++,this._saveRequested=!0,this._scheduleSave())}_scheduleSave(e=0){this._saveTimer&&clearTimeout(this._saveTimer),this._saveTimer=setTimeout(()=>{if(this._saveTimer=null,e<$.MAX_SAVE_DEFERRALS&&this.canvas?.isGestureActive?.()){this._scheduleSave(e+1);return}this._save()},500)}_canSaveProject(e){return!!this._backend&&!this._storageClosed&&e!==this._unsavableProjectId&&e!==this._deletingProject&&!this._projectGone&&this._ownsProject(e)&&this._projectLock===this._contentLock}_scheduleSaveRetry(){if(this._saveRetryTimer||!this._autosave||!this.isConnected)return;const e=this._saveRetryDelay;this._saveRetryDelay=Math.min(e*2,$.SAVE_RETRY_MAX),this._saveRetryTimer=setTimeout(()=>{this._saveRetryTimer=null;const t=this._currentProject?.id;if(!this._dirty||!t||this._stranded||!this._canSaveProject(t)){this._clearSaveError();return}if(this._projectLoads>0){this._scheduleSaveRetry();return}!this._saveTimer&&!this._savePromise&&this._save()},e)}_cancelSaveRetry(){this._saveRetryTimer&&clearTimeout(this._saveRetryTimer),this._saveRetryTimer=null,this._saveRetryDelay=$.SAVE_RETRY_MIN}_clearSaveError(){this._saveError=!1,this._lastSaveError=null,this._cancelSaveRetry()}_retrySaveNow(){!this._dirty||!this._autosave||this._projectLoads>0||(this._cancelSaveRetry(),this._savePromise||this._scheduleSave())}_userPresets(){return(this._library?.presets??[]).map(lh)}_syncTipImage(){const e=this._state.brush.tip,t=e.shape==="image"?e.imageId??null:null;t&&this._library?.ensureTip(t)}_trackBrushOp(e){const t=e.finally(()=>this._brushOps.delete(t));return this._brushOps.add(t),t}_brushStorageMessage(e){return e instanceof ae?"There is no room to store brushes. Delete some projects or brushes to make room.":"Could not store the brush."}async _defineBrush(e,t){const s=this._library;if(!s)return"Custom brushes are not available here.";try{const[i]=await this._trackBrushOp(s.add([{name:t,mask:e,descriptor:this._state.brush}]));return this._buildContextValue().selectPreset(i.id),null}catch(i){return console.error("Could not define the brush:",i),this._brushStorageMessage(i)}}_importBrushFile(e,t){return this._trackBrushOp(this._readAndStoreBrushFile(e,t))}async _readAndStoreBrushFile(e,t){const s=this._library;if(!s)return{ok:!1,message:"Custom brushes are not available here."};if(e.size>hu)return{ok:!1,message:"That file is too large to import."};try{const i=await Xh(await e.arrayBuffer(),{onProgress:t&&(a=>t(.9*a))}),r=e.name.replace(/\.[^.]*$/,"").replace(/\s+/g," ").trim().slice(0,60)||"Imported brushes";await s.importBrushes({group:r,tips:i.tips,brushes:i.brushes,patterns:i.patterns}),t?.(1),Wh(e.name,i.unmapped);const n=i.brushes.length;return{ok:!0,message:`Imported ${n} brush${n===1?"":"es"}${i.skipped>0?`, ${i.skipped} skipped`:""}`}}catch(i){return i instanceof B?{ok:!1,message:i.message}:i instanceof ae?{ok:!1,message:this._brushStorageMessage(i)}:(console.error("Could not import the brushes:",i),{ok:!1,message:i instanceof DOMException&&i.name==="NotReadableError"?"That file could not be read.":"Could not import the brushes."})}}_updateBrush(e){this._state={...this._state,brush:{...this._state.brush,...e},isPresetModified:!0},this._markDirty("setting")}_updateStampSize(e){const t=qr(e,this._state.stampSize);t!==this._state.stampSize&&(this._state={...this._state,stampSize:t},this._markDirty("setting"))}_planHistorySave(e,t){const s=this._savedHistory;let i=this._historyNeedsRewrite||this._trackedProjectId!==e||!this._backend?.history.updateEntries;if(!i){let n=-1,a=!1;for(const o of t){const h=s.get(o);if(!h)a=!0;else if(a||h.index<=n){i=!0;break}else n=h.index}}if(i)return{rewrite:i,remove:[...s],add:t,firstIndex:0};const r=new Set(t);return{rewrite:i,remove:[...s].filter(([n])=>!r.has(n)),add:t.filter(n=>!s.has(n)),firstIndex:this._nextHistoryRecordIndex}}_recordSavedHistory(e,t,s){this._trackedProjectId=e,t.rewrite&&this._savedHistory.clear();for(const[i]of t.remove)this._savedHistory.delete(i);t.add.forEach((i,r)=>{const n=new Set;xe(s[r].entry,n),this._savedHistory.set(i,{index:s[r].index,blobRefs:[...n]})}),this._nextHistoryRecordIndex=t.firstIndex+t.add.length,this._historyNeedsRewrite=!1}_trackLoadedProject(e,t,s,i=new Map,r=[],n=t.length-1){this._trackedProjectId=e,this._trackingGeneration++,this._savedHistory=new Map(t.map((c,l)=>{const d=new Set;return xe(s[l].entry,d),[c,{index:s[l].index,blobRefs:[...d]}]}));const a=[...s,...r],h=new Set(a.map(c=>c.index)).size===a.length&&a.every(c=>Number.isSafeInteger(c.index));for(const c of h?r:[])this._savedHistory.set({type:"dropped"},{index:c.index,blobRefs:ou(c)});this._nextHistoryRecordIndex=a.reduce((c,l)=>Math.max(c,Number.isSafeInteger(l.index)?l.index+1:c),0),this._historyNeedsRewrite=!h&&r.length>0,this._savedLayerBlobs=new Map([...i].map(([c,l])=>{const d=this._state.layers.find(u=>u.id===c);return[c,{...l,rev:d?this.canvas?.getLayerRevision(c)??null:null,canvas:d?.canvas}]})),this._savedContentVersion=-1,this._storedContentVersion=this._contentVersion,this._storedHistoryTop=n>=0?t[n]??null:null,this._savedThumbKey=null,this._unsavedWork=!1}async _enterProject(e,t,s=!1){const i=++this._enterGeneration,r=()=>i!==this._enterGeneration;this._enterAbort?.abort();const n=this._enterAbort=new AbortController;let a=null;this._projectLoads++,this._keptElsewhere=!1,this._stranded=!1,this._keepError="",this._overlayProjects=!1,this._projectGone=!1,this._noCanvas=!1,this._saveFailed=!1,this._clearSaveError();try{if(this._backendReopen&&await this._backendReopen,r())return;if(this._storageClosed){this._currentProject=e,this._readOnly=!0;return}if(this._currentProject=e,await this._lockProject(e.id)?await this._saveSettled(e.id,n.signal):this._readOnly=!0,r())return;let o=!1;try{await t()}catch(c){if(r())return;if(s){console.error("Failed to open the project:",c),this._unsavableProjectId=e.id;try{await this._resetToFreshProject()}catch(l){console.error("Could not start a blank document:",l),o=!0}}else{console.error("Could not make the document:",c),a=c,this.embedded||(this._notice=c instanceof RangeError?"Couldn't make a canvas that large.":"Couldn't make the document.");try{await this._resetToFreshProject()}catch(l){console.error("Could not start a blank document:",l),o=!0}}}if(r())return;const h=this._currentProject??e;h.id!==e.id&&await this._lockProject(h.id),this._contentLock=this._projectLock,this._noCanvas=o,this._readOnly=o||!this._ownsProject(h.id)}finally{this._projectLoads--,r()||(this._opening=!1),this._projectLoads===0&&this._dirty&&!this._saveTimer&&!this._savePromise&&this._scheduleSave()}if(!r()&&(this._markSaved(),this._rememberTabProject(),a))throw a}_readTabProject(){try{return sessionStorage.getItem("ketchup-tab-project")}catch{return null}}_rememberTabProject(){if(!(this.embedded||!this._currentProject))try{sessionStorage.setItem("ketchup-tab-project",this._currentProject.id)}catch{}}async _beginSwitch(e){return this._switching=!0,this._notice="",this.canvas?.cancelGesture(),this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait(),this._stranded?(this._endSwitch(e),!1):this._hasUnsavedWork()&&e===this._switchRequest&&(!await this._confirm({message:"Changes couldn't be saved — discard them?",confirmLabel:"Discard",danger:!0})||this._stranded)?(this._endSwitch(e),!1):e===this._switchRequest}_endSwitch(e){e===this._switchRequest&&(this._switching=!1)}get _locks(){return this.embedded?null:navigator.locks??null}_hasUnsavedWork(){return this._dirty&&this._contentVersion!==this._storedContentVersion||!!this.canvas?.hasPendingText?.()}_ownsProject(e){return!this._locks||this._projectLock?.id===e}_lockProject(e,t="try"){if(this._projectLock?.id===e)return this._cancelLockRequests(),Promise.resolve(!0);this._releaseProjectLock();const s=this._locks;if(!s)return Promise.resolve(!0);this._listenToTabs();const i=this._lockRequest;let r={ifAvailable:!0};t==="steal"?r={steal:!0}:t==="wait"&&(this._lockWait=new AbortController,r={signal:this._lockWait.signal});let n=null,a=Promise.resolve();const h=(t==="steal"?Promise.race([this._lockAsked,new Promise(c=>setTimeout(c,$.STEAL_WAIT))]):this._lockAsked).then(()=>new Promise(c=>{if(i!==this._lockRequest){c(!1);return}a=s.request(`ketchup-project:${e}`,r,l=>{if(!(!l||i!==this._lockRequest))return c(!0),this._tabs?.postMessage({type:"taken",id:e}),new Promise(d=>{n={id:e,release:d},this._projectLock=n})}),a.then(()=>c(!1),()=>{c(!1),!(!n||this._projectLock!==n)&&(this._projectLock=null,this._readOnly=!0,this.canvas?.clearSelection({keepCrop:!0}),this._stranded=this._hasUnsavedWork())})}));return this._lockAsked=h.then(()=>a).catch(()=>{}),h}_cancelLockRequests(){this._lockRequest++,this._lockWait?.abort(),this._lockWait=null}_releaseProjectLock(){this._cancelLockRequests(),this._projectLock?.release(),this._projectLock=null}async _saveSettled(e,t){const s=this._locks;if(!s)return;const i=`ketchup-save:${e}`;await s.request(i,{ifAvailable:!0},r=>!!r)||t.aborted||(this._readOnly=!0,this._opening=!0,await s.request(i,{signal:t},()=>{}).catch(()=>{}))}_listenToTabs(){this._tabs||typeof BroadcastChannel>"u"||(this._tabs=new BroadcastChannel("ketchup-projects"),this._tabs.addEventListener("message",e=>{const{type:t,id:s,ask:i}=e.data??{};t==="release"&&typeof i=="string"&&this._keptAsks.has(i)||(t==="release"&&this._projectLock?.id===s?this._handOver(s,i):t==="release"&&this._handingOver&&this._currentProject?.id===s?(this._handOverAgain=!0,this._handOverAgainAsks.push(typeof i=="string"?i:void 0)):t==="projects"?this._refreshProjects():t==="brushes"?this._library?.reload():t==="free"&&this._readOnly&&!this._projectLock&&this._currentProject?.id===s&&!this._detached&&document.visibilityState==="visible"&&this._editHere(!1))}))}async _projectWanted(e){const t=this._locks?.query?.bind(this._locks);if(!t)return!0;try{for(let s=0;s<6;s++){const{pending:i=[]}=await t();if(i.some(r=>r.name===`ketchup-project:${e}`))return!0;await new Promise(r=>setTimeout(r,100))}return!1}catch{return!0}}async _deleteUnlessOpenElsewhere(e){const t=this._locks,s=async()=>(await this._projectService.deleteProject(e),!0);return!t||this._projectLock?.id===e?s():t.request(`ketchup-project:${e}`,{ifAvailable:!0},i=>i?s():!1)}_announceSavedThrottled(){if(this.embedded||this._saveAnnounceTimer!==void 0)return;const e=Math.max(0,this._lastSaveAnnounce+5e3-Date.now());this._saveAnnounceTimer=setTimeout(()=>{this._saveAnnounceTimer=void 0,this._lastSaveAnnounce=Date.now(),this._announceProjects()},e)}_announceProjects(){this._tabs?.postMessage({type:"projects"})}async _refreshProjects(){if(!this._backend)return;let e;try{e=await this._backend.projects.list()}catch{return}this._projectList=e;const t=this._currentProject;if(!t||this._projectLoads>0||this.embedded)return;const s=e.find(i=>i.id===t.id);s?s.name!==t.name&&(this._currentProject={...t,name:s.name}):await Promise.resolve().then(()=>this._backend?.projects.get(t.id)).catch(()=>t)||this._onProjectGone(t.id)}async _onProjectGone(e){if(!(this._currentProject?.id!==e||this._projectLoads>0||this._projectGone||this.embedded||this._deletingProject===e)){if(this.canvas?.clearSelection({keepCrop:!0}),this._projectGone=!0,this._stranded=this._hasUnsavedWork(),this._stranded){this._readOnly=!0;return}await this._openAfterGone()}}async _openAfterGone(e){const t=this._backend;if(!t)return;const s=e??this._currentProject?.name??"The project",i=await t.projects.list();if(this._projectList=i,this._notice=`"${s}" was deleted in another tab.`,i.length>0){const r=i[0];await this._enterProject(r,()=>this._loadProject(r.id),!0)}else{const r=await t.projects.create({name:"Untitled",thumbnailRef:null});this._projectList=[r];try{await this._enterProject(r,async()=>{await this._resetToFreshProject(),this.canvas?.resetView()})}finally{this._announceProjects()}this._markDirty()}}async _handOver(e,t){if(this._tabs?.postMessage({type:"releasing",id:e,ask:t}),this._handingOver||(this._handOverAsks=[]),typeof t=="string"&&this._handOverAsks.push(t),!this._handingOver){this._handingOver=!0,this._handOverAgain=!1,this._handOverAgainAsks=[],this._confirmDialog?.dismiss();try{const s=this._projectLock;if(this._readOnly=!0,this.canvas?.clearSelection({keepCrop:!0}),(this._dirty||this._savePromise)&&await this._flushPendingSaveAndWait(),!s||this._projectLock!==s)return;if(this._dirty&&this._saveFailed){this._readOnly=this._noCanvas;for(const r of this._handOverAsks)this._keptAsks.add(r);for(const r of this._keptAsks)this._keptAsks.size>32&&this._keptAsks.delete(r);this._tabs?.postMessage({type:"kept",id:e,asks:this._handOverAsks});return}if(!await this._projectWanted(e)){this._projectLock===s&&(this._readOnly=this._noCanvas,this._tabs?.postMessage({type:"stayed",id:e,asks:this._handOverAsks}));return}if(this._projectLock!==s)return;const i=this._contentLock===s&&this._projectLoads===0&&!this._stranded;if(this._releaseProjectLock(),i&&!this._projectLock&&this._readOnly&&this._currentProject?.id===e&&!this._claiming&&!this._detached&&!this._updateRequired&&await this._lockProject(e)&&this._currentProject?.id===e){if(this._detached){this._releaseProjectLock();return}this._contentLock=this._projectLock,this._readOnly=this._noCanvas}}finally{this._handingOver=!1;const s=this._handOverAgain,i=this._handOverAgainAsks;if(this._handOverAgain=!1,this._handOverAgainAsks=[],this._readOnly&&this._dismissDialogs(),s&&this._projectLock?.id===e)for(const r of i)this._handOver(e,r)}}}async _editHere(e){const t=this._currentProject;if(!t||!this._readOnly||this._projectLock||this._claiming||this._updateRequired)return;if(!e&&(this._stranded||this._hasUnsavedWork())){this._stranded=!0;return}this._claiming=!0,this._claimCancelled=!1,this._stealing=!1;let s;this._claimEnd=new Promise(i=>{s=i}),e&&(this._keptElsewhere=!1);try{this._savePromise&&await this._savePromise,e&&(this._waitingForTab=!0);let i;try{i=await this._lockProject(t.id)}catch(n){throw this._waitingForTab=!1,n}if(!i&&e&&this._currentProject?.id===t.id&&!this._claimCancelled)try{i=await this._takeOver(t.id)}finally{this._waitingForTab=!1}else this._waitingForTab=!1;if(!i||this._claimCancelled)return;if(this._backendReopen&&await this._backendReopen,this._detached||this._updateRequired){this._ownsProject(t.id)&&(this._releaseProjectLock(),this._announceFree(t.id));return}const r=this._currentProject;if(r?.id!==t.id){this._ownsProject(t.id)&&this._releaseProjectLock();return}await this._enterProject(r,()=>this._loadProject(r.id),!0)}finally{this._stealing=!1,this._claiming=!1,s()}}_takeOver(e){const t=this._tabs;return t?new Promise(s=>{let i=!1,r=!1,n=!1,a=0,o,h;const c=Math.random().toString(36).slice(2),l=p=>p.ask===void 0&&!Array.isArray(p.asks)||p.ask===c||Array.isArray(p.asks)&&p.asks.includes(c),d=()=>{clearTimeout(o),clearTimeout(h),this._otherTabSilent=!1,this._forceTakeOver=null},u=p=>{if(p.data?.id===e){if(p.data.type==="releasing"){if(r||i||!l(p.data))return;r=!0,clearTimeout(o),n||(o=setTimeout(m,15e3))}else if(p.data.type==="stayed"){if(i||!l(p.data)||a>=$.MAX_REASKS)return;a++,clearTimeout(h),h=setTimeout(()=>t.postMessage({type:"release",id:e,ask:c}),$.REASK_WAIT)}else if(p.data.type==="taken")i||this._cancelLockRequests();else if(p.data.type==="kept"){if(i||!l(p.data))return;this._keptElsewhere=!0,this._cancelLockRequests()}}},_=p=>{d(),t.removeEventListener("message",u),s(p)};t.addEventListener("message",u),this._lockProject(e,"wait").then(p=>{i||_(p)});const f=this._lockRequest,m=()=>{f===this._lockRequest&&(n=!0,this._otherTabSilent=!0,this._forceTakeOver=()=>{i=!0,this._stealing=!0,d(),this._lockProject(e,"steal").then(_)})};o=setTimeout(m,2e3),t.postMessage({type:"release",id:e,ask:c})}):(this._stealing=!0,this._lockProject(e,"steal"))}async _keepAsNewProject(){const e=this._currentProject;if(this._claiming&&this._claimEnd&&(this._claimCancelled=!0,this._stealing||this._cancelLockRequests(),await this._claimEnd),!e||!this._backend||this._claiming||!this._stranded){e&&this._readOnly&&this._projectLock?.id===e.id&&this._releaseProjectLock();return}this._claiming=!0,this._keeping=!0,this._keepError="";let t=null,s=!1;try{if(this._savePromise&&await this._savePromise,!this._stranded)return;let i;try{i=await this._backend.projects.create({name:`${e.name} (copy)`,thumbnailRef:null})}catch(a){throw this._keepError=a instanceof ae?"Storage is full, so the copy could not be saved.":"The copy could not be saved.",a}const r=this._projectGone,n=()=>{this._currentProject=e,this._projectGone=r,this._releaseProjectLock(),this._contentLock=null,this._clearSaveError(),this._saveFailed=!1,this._stranded=!0,this._storedContentVersion=-1,this._dirty=!0,this._trackedProjectId=null,this._trackingGeneration++,this._rememberTabProject(),this._projectService?.deleteProject(i.id).catch(()=>{}),this._backend?.projects.list().then(a=>{this._projectList=a}).catch(()=>{})};if(t=n,!await this._lockProject(i.id)){this._projectService?.deleteProject(i.id).catch(()=>{});return}if(this._contentLock=this._projectLock,await this._carryStamps(e.id,i.id),this._currentProject=i,this._projectList=await this._backend.projects.list().catch(()=>[i,...this._projectList]),this._trackedProjectId=null,this._trackingGeneration++,this._savedHistory=new Map,this._nextHistoryRecordIndex=0,this._savedLayerBlobs=new Map,this._savedContentVersion=-1,this._savedThumbKey=null,this._projectGone=!1,this._saveFailed=!1,this._keepError="",this._markDirty(),await this._flushPendingSaveAndWait(),this._hasUnsavedWork()){this._keepError=this._lastSaveError instanceof ae?"Storage is full, so the copy could not be saved.":"The copy could not be saved.",n();return}s=!0,this._stranded=!1,this._readOnly=!1,this._overlayProjects=!1,this._rememberTabProject(),this._announceProjects()}catch(i){console.error("Could not keep the work as a new project:",i),t&&!s&&(this._keepError||="The copy could not be saved.",t())}finally{this._claiming=!1,this._keeping=!1,this._dirty&&this._autosave&&!this._stranded&&this._scheduleSave()}}async _holdsProjectLock(e){const t=this._locks;if(!t)return!0;if(this._projectLock?.id!==e)return!1;if(!t.query)return!0;try{const{held:s=[]}=await t.query(),i=s.find(r=>r.name===`ketchup-save:${e}`)?.clientId;return!!i&&s.some(r=>r.name===`ketchup-project:${e}`&&r.clientId===i)}catch{return!0}}_holdingSaveLock(e,t){const s=this._locks;return s?s.request(`ketchup-save:${e}`,{},t):t()}_renderThumbnail(e){const t=Math.min(1,$.THUMBNAIL_SIZE/Math.max(e.width,e.height,1)),s=document.createElement("canvas");return s.width=Math.max(1,Math.round(e.width*t)),s.height=Math.max(1,Math.round(e.height*t)),s.getContext("2d").drawImage(e,0,0,s.width,s.height),s}async _save(e=!1){if(this._savePromise)return e&&(this._forceFlushNextSave=!0,this._wakeSaveSleep?.()),this._dirty&&(this._saveRequested=!0),this._savePromise;if(!this._currentProject||!this._dirty||this._projectLoads>0)return;const t=this._currentProject.id;if(!this._canSaveProject(t)){this._saveError&&this._clearSaveError();return}const s=this._enterGeneration;this._savePromise=(async()=>{this._saveInProgress=!0,this._saveFailed=!1,this._backendReopen&&(await this._backendReopen,this._saveError&&!this._canSaveProject(t)&&this._clearSaveError());let i=e,r=0,n=null;try{for(;this._currentProject?.id===t&&this._dirty&&this._projectLoads===0&&this._ownsProject(t)&&this._projectLock===this._contentLock;){const a=t,o=this._dirtyVersion,h=this._contentVersion;let c=!this.canvas?.isGestureActive?.();const l=Date.now(),d=this._forceFlushNextSave;this._forceFlushNextSave=!1,this._saveRequested=!1;const u=i||d;this._unsavedWork&&(this._unsavedWork=!1,this._saving=!0);const _={activeTool:this._state.activeTool,strokeColor:this._state.strokeColor,fillColor:this._state.fillColor,useFill:this._state.useFill,brushSize:this._state.brush.size,stampSize:this._state.stampSize,opacity:this._state.brush.opacity,flow:this._state.brush.flow,hardness:this._state.brush.hardness,spacing:this._state.brush.spacing,pressureSize:this._state.brush.pressureSize,pressureOpacity:this._state.brush.pressureOpacity,pressureCurve:this._state.brush.pressureCurve,tip:{...this._state.brush.tip},ink:{...this._state.brush.ink},dynamics:{...this._state.brush.dynamics},jitter:{...this._state.brush.jitter},scatter:{...this._state.brush.scatter},color:{...this._state.brush.color},fade:{...this._state.brush.fade},texture:{...this._state.brush.texture},blendMode:this._state.brush.blendMode,airbrush:this._state.brush.airbrush,airbrushRate:this._state.brush.airbrushRate,activePreset:this._state.activePreset,isPresetModified:this._state.isPresetModified,cropAspectRatio:this._state.cropAspectRatio,fontFamily:this._state.fontFamily,fontSize:this._state.fontSize,fontBold:this._state.fontBold,fontItalic:this._state.fontItalic,eyedropperSampleAll:this._state.eyedropperSampleAll,stabilizer:this._state.stabilizer,childMode:this._state.childMode},f=this._state.documentWidth,m=this._state.documentHeight,p=this._state.activeLayerId,g=this._desktopLayersPanelOpen??this._state.layersPanelOpen,y=this.canvas?.getFloatKey()??null,w=!y&&h===this._savedContentVersion&&this._trackedProjectId===a&&this._state.layers.every(C=>this._savedLayerBlobs.has(C.id)),x=(C,j=this.canvas?.getFloatKey()??null)=>{const X=this.canvas?.getLayerRevision(C)??null;return j&&C===j.layerId&&X!==null?`${X}|float:${j.key}`:X},b=this._state.layers.map(C=>x(C.id,y)),S=this._state.layers.map(C=>C.canvas),E=this._state.layers.map((C,j)=>{const X=this._savedLayerBlobs.get(C.id);return b[j]!==null&&!!X&&X.rev===b[j]&&X.canvas===C.canvas&&this._trackedProjectId===a}),D=C=>{const j=this.canvas?.previewFloatCommit()??null;if(!j||j.layerId!==C)throw new Error(`Could not read the float on layer ${C}`);return j.imageData},L=this._state.layers.map((C,j)=>{const X={id:C.id,name:C.name,visible:C.visible,opacity:C.opacity,blendMode:C.blendMode,alphaLocked:C.alphaLocked===!0};if(w||E[j])return{...X,imageData:null};if(y&&C.id===y.layerId)return{...X,imageData:D(C.id)};const Ut=C.canvas.getContext("2d").getImageData(0,0,C.canvas.width,C.canvas.height);return{...X,imageData:Ut}}),I=L.map(C=>C.imageData?ni(C.imageData):this._savedLayerBlobs.get(C.id).hash),k=this.canvas?.getViewport()??{zoom:1,panX:0,panY:0},M=this.canvas?.getViewportSize()??null,P=y?this.canvas.getHistoryAsCommitted():null,R=P?.history??this.canvas?.getHistory()??[],O=P?.index??this.canvas?.getHistoryIndex()??-1,V=this._trackingGeneration,H=this._projectLock,F=this._planHistorySave(a,R),nt=F.rewrite;let vt=null;if(u){const C=new Promise(j=>{n=j});vt=this._holdingSaveLock(a,async()=>{if(this._locks&&!await this._holdsProjectLock(a))return!1;const j=await C;return j?j():!1}),vt.catch(()=>{})}const G=this._backend.blobs,[Ht,Xt,Wt]=await Promise.all([this._backend.state.get(a),this._backend.projects.get(a),nt?this._backend.history.getEntries(a):Promise.resolve([])]);if(!Xt){if(this.embedded)throw new Error("The project is no longer in storage");this._clearSaveError(),this._onProjectGone(a);break}const Mt=Ht?.layers.map(C=>C.imageBlobRef)??[],pt=Xt.thumbnailRef??null,st=[],ct={get:C=>G.get(C),delete:C=>G.delete(C),deleteMany:C=>G.deleteMany(C),gc:G.gc?C=>G.gc(C):void 0,async put(C){const j=await G.put(C);return st.push(j),j}};let N,K,Ct=!1;try{N=await Promise.all(L.map((C,j)=>{const X=this._savedLayerBlobs.get(C.id);if(X&&X.hash===I[j]&&Mt.includes(X.blobRef))return{id:C.id,name:C.name,visible:C.visible,opacity:C.opacity,blendMode:C.blendMode,alphaLocked:C.alphaLocked,imageBlobRef:X.blobRef};let dt=C.imageData;if(!dt){const Ut=this._state.layers.find(Ka=>Ka.id===C.id)?.canvas;if(!Ut)throw new Error(`Layer ${C.id} disappeared during save`);const Vt=this.canvas?.getFloatKey()??null;if(!(Ut===S[j]&&(b[j]!==null?x(C.id,Vt)===b[j]:this._contentVersion===h&&Vt?.layerId!==C.id)))return Ct=!0,null;dt=Vt?.layerId===C.id?D(C.id):Ut.getContext("2d").getImageData(0,0,Ut.width,Ut.height),I[j]=ni(dt),c=!1}return Rc(C,dt,ct)})),K=await Promise.all(F.add.map(async(C,j)=>({projectId:a,index:F.firstIndex+j,entry:await Ac(C,ct)})))}catch(C){throw st.length>0&&G.deleteMany(st).catch(()=>{}),C}if(Ct){if(G.deleteMany(st).catch(()=>{}),n?.(null),n=null,++r>3)throw new Error("The drawing kept changing during the save");continue}const St=N,Pt={projectId:a,toolSettings:_,canvasWidth:f,canvasHeight:m,layers:St,activeLayerId:p,layersPanelOpen:g,historyIndex:O,zoom:k.zoom,panX:k.panX,panY:k.panY,viewportWidth:M?.width,viewportHeight:M?.height};let it=null;const tt=`${h}|${k.zoom},${k.panX},${k.panY}|${M?.width}x${M?.height}`,lt=!y&&pt&&tt===this._savedThumbKey;if(this.canvas?.mainCanvas&&!lt)try{it=await cr(this._renderThumbnail(this.canvas.mainCanvas))}catch{}const Yt=async()=>{let C=pt;try{if(it){if(C=await G.put(it),await this._backend.projects.update(a,{thumbnailRef:C}),this._currentProject?.id===a&&this._trackingGeneration===V){const j=this.canvas?.getViewport(),X=j?.zoom===k.zoom&&j.panX===k.panX&&j.panY===k.panY;this._savedThumbKey=c&&X?tt:null}}else await this._backend.projects.update(a,{})}catch{C!==pt&&C&&G.delete(C).catch(()=>{})}try{const j=new Set(St.map(dt=>dt.imageBlobRef)),X=Mt.filter(dt=>!j.has(dt));if(pt&&pt!==C&&X.push(pt),!nt)for(const[,dt]of F.remove)X.push(...dt.blobRefs);if(nt&&Wt.length>0){const dt=new Set;for(const Vt of Wt)xe(Vt?.entry,dt);const Ut=new Set;for(const Vt of K)xe(Vt.entry,Ut);for(const Vt of dt)Ut.has(Vt)||X.push(Vt)}X.length>0&&G.deleteMany(X).catch(()=>{})}catch(j){console.warn("Blob cleanup after save failed:",j)}},_r=async()=>{if(this._projectLock!==H||this._trackingGeneration!==V||!vt&&this._locks&&!await this._holdsProjectLock(a))return!1;const C=this._backend,j=async()=>{try{await this._backend.state.save(Pt),nt?await this._backend.history.replaceAll(a,K):(F.remove.length>0||K.length>0)&&await this._backend.history.updateEntries(a,F.remove.map(([,X])=>X.index),K)}catch(X){throw Ht&&await this._backend.state.save(Ht).catch(dt=>{console.error("Failed to rollback state after save failure:",dt)}),G.deleteMany(st).catch(()=>{}),X}};return await(C.writeTogether?C.writeTogether(j):j()),await Yt(),!0};if(!(vt?(n(_r),await vt):await this._holdingSaveLock(a,_r))){G.deleteMany(st).catch(()=>{}),this._saveError&&(this._canSaveProject(a)?this._scheduleSaveRetry():this._clearSaveError());break}if(this._currentProject?.id===a&&this._trackingGeneration===V&&(this._recordSavedHistory(a,F,K),this._savedLayerBlobs=new Map(L.map((C,j)=>[C.id,{hash:I[j],blobRef:St[j].imageBlobRef,rev:c?b[j]:null,canvas:S[j]}])),this._savedContentVersion=c?h:-1,this._storedContentVersion=h,this._storedHistoryTop=R[O]??null),this._currentProject?.id===a&&this._dirtyVersion===o&&(this._dirty=!1),this._clearSaveError(),r=0,await this._backend.projects.list().then(C=>{this._projectList=C}).catch(()=>{}),this._announceSavedThrottled(),!u&&!this._forceFlushNextSave){const C=Date.now()-l;C<1500&&await new Promise(j=>{const X=()=>{clearTimeout(dt),this._wakeSaveSleep===X&&(this._wakeSaveSleep=null),j()},dt=setTimeout(X,1500-C);this._wakeSaveSleep=X})}if(!this._saveRequested||!this._dirty)break;if(!i&&!this._forceFlushNextSave&&this.canvas?.isGestureActive?.()){this._scheduleSave();break}i=!1}}catch(a){this._currentProject?.id===t&&this._enterGeneration===s&&!this._storageClosed&&(this._saveFailed=!0,this._lastSaveError=a,this._saveError=!0,a instanceof ae||this._scheduleSaveRetry()),a instanceof ae?console.error("Storage quota exceeded. Consider deleting old projects to free space."):console.error("Save failed:",a)}finally{n?.(null),this._saving=!1,this._saveInProgress=!1,this._stranded&&!this._hasUnsavedWork()&&(this._stranded=!1)}})();try{await this._savePromise}finally{this._savePromise=null}}_isTextEntryTarget(e){for(const t of e.composedPath()){if(t===this)break;if(t instanceof HTMLElement){if(t.isContentEditable||t instanceof HTMLTextAreaElement)return!0;if(t instanceof HTMLInputElement)return!$.NON_TEXT_INPUT_TYPES.has(t.type);if(t instanceof HTMLDialogElement&&t.open)return!0}}return!1}_onCommitOpacity(e){const{layerId:t,before:s,after:i}=e.detail;this.canvas?.pushLayerOperation({type:"opacity",layerId:t,before:s,after:i}),this._markDirty()}_onCropCommit(e){const{width:t,height:s}=e.detail;this._applyDocumentDimensions(t,s),this._state={...this._state,layers:[...this._state.layers]},this._markDirty()}async _resetToFreshProject(e=800,t=600,s="#ffffff"){this.canvas?.clearSelection();const i=e,r=t,n=this._layerCounter;this._layerCounter=0;const a=this._createLayer(i,r);if(!a.canvas.getContext("2d"))throw this._layerCounter=n,new RangeError(`This browser cannot make a ${i}×${r} canvas`);if(this._state={activeTool:"pencil",strokeColor:"#000000",fillColor:"#ff0000",useFill:!1,brush:Is(),activePreset:"round",isPresetModified:!1,stampImage:null,activeStampId:null,stampSize:Ei,layers:[a],activeLayerId:a.id,layersPanelOpen:!this._isMobile,documentWidth:i,documentHeight:r,cropAspectRatio:"free",fontFamily:"sans-serif",fontSize:24,fontBold:!1,fontItalic:!1,eyedropperSampleAll:!0,stabilizer:0,childMode:!1},this._layoutWidth>0&&this._updateMobileLayout(this._layoutWidth),await this.updateComplete,this.canvas?.setHistory([],-1),s){const o=a.canvas.getContext("2d");o.fillStyle=s,o.fillRect(0,0,a.canvas.width,a.canvas.height)}this.canvas?.composite(),this._dirty=!1,this._trackLoadedProject(this._currentProject?.id??null,[],[]),this._historyNeedsRewrite=!0,this._isMobile&&(this._desktopLayersPanelOpen=!0)}async _loadProject(e){const t=this._enterGeneration,s=()=>t!==this._enterGeneration;try{this.canvas?.clearSelection();const i=await this._backend.state.get(e);if(s())return;if(!i){if(!await this._backend.projects.get(e).catch(()=>({}))){if(s())return;await this._replaceDeletedProject(e);return}await this._resetToFreshProject();return}const r=16384;if(i.canvasWidth<=0||i.canvasWidth>r||i.canvasHeight<=0||i.canvasHeight>r){console.error("Invalid canvas dimensions in saved state:",i.canvasWidth,i.canvasHeight),await this._resetToFreshProject();return}const n=this._backend.blobs,a=await Promise.all(i.layers.map(I=>Dc(I,i.canvasWidth,i.canvasHeight,n)));if(s())return;if(a.length===0){await this._resetToFreshProject();return}const o=await this._backend.history.getEntries(e),h=o.length,c=Math.min(i.historyIndex??h-1,h-1),l=Ma();let d=0,u=c+1;for(;u>0&&(d+=Yr(o[u-1].entry),!(d>l&&u<c+1));)u--;let _=c+1;for(;_<h&&(d+=Yr(o[_].entry),!(d>l));)_++;const f=o.slice(u,_);let m=[],p=u;for(let I=0;I<f.length;I++){try{m.push(await Lc(f[I].entry,n))}catch(k){if(s())return;const M=k?.name,P=k instanceof Ne||M==="StorageNotFoundError"||M==="NotFoundError"||M==="NotReadableError";if(!(k instanceof ke||k instanceof Ca||P))throw k;console.error("Dropping undecodable history entry:",k);const R=u+I;if(R<=c)m=[],p=R+1;else{_=R;break}}if(s())return}if(s())return;u=p;const g=o.slice(u,_),y=new Map(a.map((I,k)=>{const M=I.canvas.getContext("2d").getImageData(0,0,I.canvas.width,I.canvas.height);return[I.id,{hash:ni(M),blobRef:i.layers[k].imageBlobRef}]})),w=a.reduce((I,k)=>{const M=k.name.match(/^Layer (\d+)$/);return M?Math.max(I,parseInt(M[1])):I},0);this._layerCounter=w;const x=a.some(I=>I.id===i.activeLayerId)?i.activeLayerId:a[0].id,b=i.toolSettings,S=Ws({size:b.brushSize,opacity:b.opacity,flow:b.flow,hardness:b.hardness,spacing:b.spacing,pressureSize:b.pressureSize,pressureOpacity:b.pressureOpacity,pressureCurve:b.pressureCurve,tip:b.tip,ink:b.ink,dynamics:b.dynamics,jitter:b.jitter,scatter:b.scatter,color:b.color,fade:b.fade,texture:b.texture,blendMode:b.blendMode,airbrush:b.airbrush,airbrushRate:b.airbrushRate});this._state={activeTool:b.activeTool==="marker"?"pencil":b.activeTool,strokeColor:b.strokeColor,fillColor:b.fillColor,useFill:b.useFill,brush:S,activePreset:b.activePreset??"round",isPresetModified:b.isPresetModified??!1,stampImage:null,activeStampId:null,stampSize:qr(b.stampSize),layers:a,activeLayerId:x,layersPanelOpen:i.layersPanelOpen&&!this._isMobile,documentWidth:i.canvasWidth,documentHeight:i.canvasHeight,cropAspectRatio:b.cropAspectRatio??"free",fontFamily:b.fontFamily??"sans-serif",fontSize:b.fontSize??24,fontBold:b.fontBold??!1,fontItalic:b.fontItalic??!1,eyedropperSampleAll:b.eyedropperSampleAll??!0,stabilizer:Li(b.stabilizer),childMode:b.childMode??!1};const E=getComputedStyle(this),D=this.clientWidth-parseFloat(E.paddingLeft)-parseFloat(E.paddingRight);if(D>0&&this._updateMobileLayout(D),this._isMobile&&(this._desktopLayersPanelOpen=i.layersPanelOpen),await this.updateComplete,s())return;const L=Math.max(-1,Math.min(c-u,m.length-1));if(this.canvas?.setHistory(m,L),this._dirty=!1,this._trackLoadedProject(e,m,g,y,[...o.slice(0,u),...o.slice(_)],L),this._unsavableProjectId===e&&(this._unsavableProjectId=null),i.zoom!=null&&i.panX!=null&&i.panY!=null){const I=i.viewportWidth!=null&&i.viewportHeight!=null?{width:i.viewportWidth,height:i.viewportHeight}:void 0;this.canvas?.restoreViewport(i.zoom,i.panX,i.panY,I)}else this.canvas?.resetView()}catch(i){if(s())return;console.error("Failed to load project:",i);try{const r=await this._backend.projects.create({name:"Untitled",thumbnailRef:null});if(s()){this._projectService?.deleteProject(r.id).catch(()=>{});return}if(this._currentProject=r,this._projectList=await this._backend.projects.list(),s()){this._projectService?.deleteProject(r.id).then(()=>this._backend?.projects.list()).then(n=>{n&&(this._projectList=n)}).catch(()=>{});return}}catch(r){console.error("Could not start a new project:",r),this._unsavableProjectId=e}if(s())return;await this._resetToFreshProject()}}async _replaceDeletedProject(e){const t=this._projectList.find(s=>s.id===e)?.name??"The project";await this._openAfterGone(t)}_applyDocumentDimensions(e,t){this._state={...this._state,documentWidth:e,documentHeight:t}}_acceptPastedImage(){this.canvas?.hasExternalFloat&&this.canvas.commitTransform()}_buildContextValue(){return{state:this._state,setTool:e=>{this._state.activeTool!==e&&(this.canvas?.cancelCrop(),Xn(e)&&this.canvas?.isTransformActive()||(this.canvas?.isTransformActive()&&this.canvas.commitTransform(),this.canvas?.clearSelection())),this._state={...this._state,activeTool:e},this._markDirty("setting")},setStrokeColor:e=>{this._state={...this._state,strokeColor:e},this._markDirty("setting")},setFillColor:e=>{this._state={...this._state,fillColor:e},this._markDirty("setting")},setUseFill:e=>{this._state={...this._state,useFill:e},this._markDirty("setting")},setBrushSize:e=>{const t=Number.isNaN(e)?this._state.brush.size:e;this._updateBrush({size:Math.max(1,Math.min(150,t))})},setStampSize:e=>{this._updateStampSize(e)},setStampImage:(e,t=null)=>{this._state={...this._state,stampImage:e,activeStampId:e?t:null},this._markDirty("setting")},undo:()=>this.canvas?.undo(),redo:()=>this.canvas?.redo(),clearCanvas:(e=!1)=>{if(this._readOnly||this._handingOver||this._switching||this._replacing)return;if(!e){this.canvas?.clearCanvas();return}this.canvas?.clearSelection();const t=this._state.layers;if(t.length===0)return;const s=this._snapshotAllLayers(),i=this._state.activeLayerId,r=t[0],n=this._state.documentWidth,a=this._state.documentHeight,o=document.createElement("canvas");o.width=n,o.height=a;const h=o.getContext("2d");h.fillStyle="#ffffff",h.fillRect(0,0,n,a);const c={...r,visible:!0,opacity:1,blendMode:"normal",alphaLocked:!1,canvas:o};this._state={...this._state,layers:[c],activeLayerId:c.id};const l=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:s,afterLayers:l,previousActiveLayerId:i,afterActiveLayerId:c.id}),this._markDirty()},saveCanvas:()=>this._requestSave(),embedded:this.embedded,addLayer:e=>{this.canvas?.clearSelection();const t=this._createLayer(this._state.documentWidth,this._state.documentHeight);e&&(t.name=e,this._layerCounter--);const i=this._state.layers.findIndex(n=>n.id===this._state.activeLayerId)+1,r=[...this._state.layers];return r.splice(i,0,t),this._state={...this._state,layers:r,activeLayerId:t.id},this.canvas?.pushLayerOperation({type:"add-layer",layer:this._snapshotLayer(t),index:i}),this._markDirty(),t.id},deleteLayer:e=>{if(this._state.layers.length<=1)return;const t=this._state.layers.findIndex(a=>a.id===e);if(t===-1)return;this._acceptPastedImage(),e===this._state.activeLayerId&&this.canvas?.clearSelection();const s=this._state.layers[t],i=this._snapshotLayer(s),r=this._state.layers.filter(a=>a.id!==e),n=this._state.activeLayerId===e?r[Math.min(t,r.length-1)].id:this._state.activeLayerId;this._state={...this._state,layers:r,activeLayerId:n},this.canvas?.pushLayerOperation({type:"delete-layer",layer:i,index:t}),this._markDirty()},setActiveLayer:e=>{this._state.layers.some(t=>t.id===e)&&e!==this._state.activeLayerId&&(this.canvas?.clearSelection(),this._state={...this._state,activeLayerId:e},this._markDirty("setting"))},setLayerVisibility:(e,t)=>{const s=this._state.layers.find(n=>n.id===e);if(!s||s.visible===t)return;const i=s.visible,r=this._state.layers.map(n=>n.id===e?{...n,visible:t}:n);this._state={...this._state,layers:r},this.canvas?.pushLayerOperation({type:"visibility",layerId:e,before:i,after:t}),this._markDirty()},setLayerOpacity:(e,t)=>{if(!this._state.layers.find(a=>a.id===e))return;const i=Number.isFinite(t)?t:1,r=Math.max(0,Math.min(1,i)),n=this._state.layers.map(a=>a.id===e?{...a,opacity:r}:a);this._state={...this._state,layers:n},this._markDirty()},reorderLayer:(e,t)=>{const s=this._state.layers.findIndex(a=>a.id===e);if(s===-1||s===t)return;this._acceptPastedImage();const i=[...this._state.layers],[r]=i.splice(s,1),n=t<0?Math.max(i.length+t,0):Math.min(t,i.length);i.splice(n,0,r),this._state={...this._state,layers:i},this.canvas?.pushLayerOperation({type:"reorder",fromIndex:s,toIndex:n}),this._markDirty()},renameLayer:(e,t)=>{const s=this._state.layers.find(n=>n.id===e);if(!s||s.name===t)return;const i=s.name,r=this._state.layers.map(n=>n.id===e?{...n,name:t}:n);this._state={...this._state,layers:r},this.canvas?.pushLayerOperation({type:"rename",layerId:e,before:i,after:t}),this._markDirty()},setLayerBlendMode:(e,t)=>{const s=this._state.layers.find(n=>n.id===e);if(!s||s.blendMode===t)return;const i=s.blendMode,r=this._state.layers.map(n=>n.id===e?{...n,blendMode:t}:n);this._state={...this._state,layers:r},this.canvas?.pushLayerOperation({type:"blend-mode",layerId:e,before:i,after:t}),this._markDirty()},setLayerAlphaLocked:(e,t)=>{const s=this._state.layers.find(n=>n.id===e);if(!s||s.alphaLocked===!0===t)return;const i=s.alphaLocked===!0,r=this._state.layers.map(n=>n.id===e?{...n,alphaLocked:t}:n);this._state={...this._state,layers:r},this.canvas?.pushLayerOperation({type:"alpha-lock",layerId:e,before:i,after:t}),this._markDirty()},mergeLayerDown:e=>{const t=this._state.layers,s=t.findIndex(l=>l.id===e);if(s<=0)return;this.canvas?.clearSelection();const i=this._snapshotAllLayers(),r=this._state.activeLayerId,n=t[s-1],a=t[s],o=this._compositeLayers([n,a],null),h=t.filter(l=>l.id!==a.id).map(l=>l.id===n.id?{...l,canvas:o,opacity:1,blendMode:"normal"}:l);this._state={...this._state,layers:h,activeLayerId:n.id};const c=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:i,afterLayers:c,previousActiveLayerId:r,afterActiveLayerId:n.id}),this._markDirty()},mergeVisibleLayers:()=>{const e=this._state.layers,t=e.filter(d=>d.visible);if(t.length<2)return;this.canvas?.clearSelection();const s=this._snapshotAllLayers(),i=this._state.activeLayerId,r=t[0],n=this._compositeLayers(t,null),a=new Set(t.map(d=>d.id)),o=e.filter(d=>!a.has(d.id)||d.id===r.id).map(d=>d.id===r.id?{...d,canvas:n,opacity:1,blendMode:"normal"}:d),c=o.some(d=>d.id===i)?i:r.id;this._state={...this._state,layers:o,activeLayerId:c};const l=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:s,afterLayers:l,previousActiveLayerId:i,afterActiveLayerId:c}),this._markDirty()},flattenImage:()=>{if(this._state.layers.length<=1)return;this.canvas?.clearSelection();const e=this._snapshotAllLayers(),t=this._state.activeLayerId,s=this._state.layers.filter(o=>o.visible),i=s.length>0?s[0]:this._state.layers[0],r=this._compositeLayers(s),n={id:i.id,name:i.name,visible:!0,opacity:1,blendMode:"normal",alphaLocked:!1,canvas:r};this._state={...this._state,layers:[n],activeLayerId:n.id};const a=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:e,afterLayers:a,previousActiveLayerId:t,afterActiveLayerId:n.id}),this._markDirty()},toggleLayersPanel:()=>{this._state={...this._state,layersPanelOpen:!this._state.layersPanelOpen},this._markDirty("setting")},setCropAspectRatio:e=>{this._state={...this._state,cropAspectRatio:e},this._markDirty("setting")},setFontFamily:e=>{this._state={...this._state,fontFamily:e},this._markDirty("setting")},setFontSize:e=>{const t=Number.isFinite(e)?e:8;this._state={...this._state,fontSize:Math.max(8,Math.min(200,t))},this._markDirty("setting")},setFontBold:e=>{this._state={...this._state,fontBold:e},this._markDirty("setting")},setFontItalic:e=>{this._state={...this._state,fontItalic:e},this._markDirty("setting")},setBrush:e=>{this._updateBrush(e)},setBrushTip:e=>{this._updateBrush({tip:{...this._state.brush.tip,...e}})},setBrushInk:e=>{this._updateBrush({ink:{...this._state.brush.ink,...e}})},setBrushDynamics:e=>{this._updateBrush({dynamics:{...this._state.brush.dynamics,...e}})},setBrushJitter:e=>{this._updateBrush({jitter:{...this._state.brush.jitter,...e}})},setBrushScatter:e=>{this._updateBrush({scatter:{...this._state.brush.scatter,...e}})},setBrushColorDynamics:e=>{this._updateBrush({color:{...this._state.brush.color,...e}})},setBrushFade:e=>{this._updateBrush({fade:{...this._state.brush.fade,...e}})},setBrushTexture:e=>{this._updateBrush({texture:{...this._state.brush.texture,...e}})},selectPreset:e=>{const t=Xo(e)??this._userPresets().find(i=>i.id===e);if(!t)return;const s=t.descriptor;this._state={...this._state,brush:Oe(s),activePreset:e,isPresetModified:!1},this._markDirty("setting")},userTips:this._library?.tips??[],userPresets:this._userPresets(),defineBrushFromSelection:async()=>{const e=this.canvas?.getFloatImage();if(!e)return"Select part of the image first.";const t=_a(e);return t?this._defineBrush(t,`Brush ${this._userPresets().length+1}`):"The selection has nothing to paint with: it is all white or transparent."},defineBrushFromFile:async(e,t)=>{let s;try{s=await Uh(e)}catch{return"That file could not be read as an image."}return s?this._defineBrush(s,t?.replace(/\.[^.]*$/,"").trim()||`Brush ${this._userPresets().length+1}`):"The image has nothing to paint with: it is all white or transparent."},loadUserTips:e=>{const t=e?new Set(e):null;for(const s of this._library?.presets??[])s.tipId&&(!t||t.has(s.group??""))&&this._library.ensureTip(s.tipId)},importBrushFile:(e,t)=>this._importBrushFile(e,t),deleteUserGroup:async e=>{const t=this._library?.presets.filter(i=>i.group===e).length??0;if(!(t===0||!await this._confirm({message:`Delete the ${t===1?"brush":`${t} brushes`} imported from "${e}"? Projects that use their tips will paint with a round tip.`,confirmLabel:"Delete",danger:!0})))try{await this._trackBrushOp(Promise.resolve(this._library?.removeGroup(e)))}catch(i){this._notice=this._brushStorageMessage(i)}},selectUserTip:e=>{const t=this._library?.tipMeta(e);t&&this._updateBrush({tip:{...Q("image"),imageId:e},spacing:t.spacing})},deleteUserPreset:async e=>{const t=this._library?.presets.find(s=>s.id===e)?.tipId;if(!(t&&!this._library.presets.some(s=>s.id!==e&&s.tipId===t)&&!await this._confirm({message:"Delete this brush and its tip image? Projects that use the tip will paint with a round tip.",confirmLabel:"Delete",danger:!0})))try{await this._trackBrushOp(Promise.resolve(this._library?.removePreset(e)))}catch(s){this._notice=this._brushStorageMessage(s)}},deleteUserTip:async e=>{const t=this._library?.presets.filter(i=>i.tipId===e).length??0;if(await this._confirm({message:t>0?`Delete this tip and the ${t===1?"brush":`${t} brushes`} made from it? Projects that use it will paint with a round tip.`:"Delete this tip? Projects that use it will paint with a round tip.",confirmLabel:"Delete",danger:!0}))try{await this._trackBrushOp(Promise.resolve(this._library?.removeTip(e)))}catch(i){this._notice=this._brushStorageMessage(i)}},setEyedropperSampleAll:e=>{this._state={...this._state,eyedropperSampleAll:e},this._markDirty("setting")},setStabilizer:e=>{this._state={...this._state,stabilizer:Li(e,this._state.stabilizer)},this._markDirty("setting")},confirm:e=>this._confirm(e),canUndo:this._canUndo,canRedo:this._canRedo,currentProject:this._currentProject,projectList:this._projectList,saving:this._saving,zoom:this._viewportZoom,panX:this._viewportPanX,panY:this._viewportPanY,viewportWidth:this._viewportWidth,viewportHeight:this._viewportHeight,isMobile:this._isMobile,switchProject:e=>{if(e===this._currentProject?.id)return;const t=++this._switchRequest;(async()=>{if(!await this._beginSwitch(t))return;const i=this._projectList.find(r=>r.id===e);i&&await this._enterProject(i,()=>this._loadProject(e),!0)})().catch(i=>console.error("Switch project failed:",i)).finally(()=>this._endSwitch(t))},createProject:(e,t,s)=>{const i=++this._switchRequest;(async()=>{if(!await this._beginSwitch(i))return;const n=await this._backend.projects.create({name:e,thumbnailRef:null});if(i!==this._switchRequest){await this._projectService.deleteProject(n.id).catch(()=>{}),this._announceProjects(),this._projectList=await this._backend.projects.list();return}try{await this._enterProject(n,async()=>{this._projectList=await this._backend.projects.list(),await this._resetToFreshProject(t,s),this.canvas?.resetView()})}finally{this._announceProjects()}this._markDirty()})().catch(n=>console.error("Create project failed:",n)).finally(()=>this._endSwitch(i))},deleteProject:e=>{const t=e===this._currentProject?.id,s=t?++this._switchRequest:this._switchRequest;(async()=>{if(t){if(!await this._beginSwitch(s))return}else this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait();this._deletingProject=e;try{if(!await this._deleteUnlessOpenElsewhere(e)){this._notice="That project is open in another tab. Close it there to delete it.";return}if(this._announceProjects(),!t&&this._saveError&&this._retrySaveNow(),this._projectList=await this._backend.projects.list(),s!==this._switchRequest&&t)return;if(e===this._currentProject?.id)if(this._projectList.length>0){const r=this._projectList[0];await this._enterProject(r,()=>this._loadProject(r.id),!0)}else{const r=await this._backend.projects.create({name:"Untitled",thumbnailRef:null});this._projectList=[r],this._announceProjects(),await this._enterProject(r,async()=>{await this._resetToFreshProject(),this.canvas?.resetView()}),this._markDirty()}}finally{this._deletingProject===e&&(this._deletingProject=null)}})().catch(r=>console.error("Delete project failed:",r)).finally(()=>{t&&this._endSwitch(s)})},renameProject:(e,t)=>{(async()=>{const i=await this._backend.projects.update(e,{name:t});this._currentProject?.id===e&&(this._currentProject=i),this._projectList=await this._backend.projects.list(),this._announceProjects()})().catch(i=>console.error("Rename project failed:",i))},transformActive:this.canvas?.isTransformActive()??!1,getTransformValues:()=>this.canvas?.getTransformValues()??null,setTransformValue:(e,t)=>this.canvas?.setTransformValue(e,t),setChildMode:e=>{this._state={...this._state,childMode:e},!e&&this._layoutWidth>0&&this._updateMobileLayout(this._layoutWidth),e&&!Oc.has(this._state.activeTool)&&(this.canvas?.isTransformActive()&&this.canvas.commitTransform(),this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"pencil"}),this._markDirty("setting")}}}updated(e){(e.has("_readOnly")||e.has("_switching"))&&(this._readOnly&&!this._handingOver||this._switching)&&(this._readOnly&&!this._handingOver?this._dismissDialogs():this.canvas?.dismissResizeDialog()),(e.has("_readOnly")||e.has("_handingOver")||e.has("_replacing"))&&(this._readOnly||this._handingOver||this._replacing)&&this._confirmDialog?.dismiss()}willUpdate(){this._updateRequired&&!this._readOnly&&(this._readOnly=!0),this._syncTipImage(),this._provider.setValue(this._buildContextValue()),this.toggleAttribute("mobile",this._isMobile),this._overlayProjects&&!this._readOnly&&(this._overlayProjects=!1)}_onHistoryChange(e){if(this._canUndo=e.detail.canUndo,this._canRedo=e.detail.canRedo,e.detail.stackChanged!==!1){const t=e.detail.committedAsPreviewed?this.canvas?.getHistory():null,s=t?.[this.canvas.getHistoryIndex()],i=!!t&&this._storedContentVersion===this._contentVersion&&(s??null)===this._storedHistoryTop;this._markDirty(),i&&(this._storedContentVersion=this._contentVersion)}this._reportModified()}whenReady(){return this._ready}openImage(e,t={}){return this._replaceDocumentInTurn(async()=>{const s=await createImageBitmap(e);try{Nn(s.width,s.height),await this._replaceDocument(s.width,s.height,null,t.name??"Untitled",i=>i.canvas.getContext("2d").drawImage(s,0,0),`This browser cannot open a ${s.width}×${s.height} image`)}finally{s.close()}})}newDocument(e,t,s={}){return this._replaceDocumentInTurn(()=>this._replaceDocument(e,t,s.background===void 0?"#ffffff":s.background,s.name??"Untitled"))}async exportImage(e={}){const t=e.type??"image/png",s=e.background!==void 0?e.background:t==="image/jpeg"?"#ffffff":null,i=this._documentReplacement.then(async()=>(await this._ready,this.canvas.clearSelection({keepCrop:!0}),{canvas:this.canvas.renderFlattened(s),mark:this._markDocument()}));this._documentReplacement=i.catch(()=>{});const{canvas:r,mark:n}=await i;return new Promise((a,o)=>{r.toBlob(h=>{if(!h){o(new Error(`Could not encode the image as ${t}`));return}this._exportedDocument=n,this._exportMarks.set(h,n),a(h)},t,e.quality)})}get modified(){if(!this.canvas)return!1;if(this.canvas.isTransformActive()||this.canvas.hasPendingText())return!0;const e=this._historyTop();return e!==this._savedDocument.top?!0:e===null&&this.canvas.getHistoryTrimmedCount()!==this._savedDocument.trimmed}markSaved(e){const t=e?this._exportMarks.get(e):void 0;t&&t.generation!==this._documentGeneration||(this._savedDocument=t??this._exportedDocument??this._markDocument(),this._exportedDocument=null,this._reportModified())}async _showWholeDocument(){await this.updateComplete;const e=this.canvas;e&&(await e.updateComplete,e.resetView())}_historyTop(){const e=this.canvas?.getHistoryIndex()??-1;return e>=0?this.canvas.getHistory()[e]??null:null}_markDocument(){return{top:this._historyTop(),trimmed:this.canvas?.getHistoryTrimmedCount()??0,generation:this._documentGeneration}}_markSaved(){this._confirmDialog?.dismiss(),this._documentGeneration++,this._savedDocument=this._markDocument(),this._exportedDocument=null,this._reportModified()}_replaceDocumentInTurn(e){const t=this._documentReplacement.then(async()=>{await this._ready,await e(),await this._showWholeDocument(),this._markDirty(),this._markSaved()});return this._documentReplacement=t.catch(()=>{}),t}_reportModified(){const e=this.modified;e!==this._lastReportedModified&&(this._lastReportedModified=e,this.dispatchEvent(new CustomEvent("modified-change",{detail:{modified:e},bubbles:!0,composed:!0})))}_requestSave(){if(!this.embedded){this.canvas?.commitPendingText(),this.canvas?.saveCanvas();return}this.canvas?.clearSelection({keepCrop:!0}),this.dispatchEvent(new CustomEvent("save-request",{bubbles:!0,composed:!0}))}async _replaceDocument(e,t,s,i,r,n){e=Math.round(e),t=Math.round(t),Nn(e,t);{const a=document.createElement("canvas");a.width=e,a.height=t;const o=!!a.getContext("2d");if(a.width=a.height=0,!o)throw new RangeError(n??`This browser cannot make a ${e}×${t} canvas`)}this._replacing=!0,this._confirmDialog?.dismiss();try{await this._replaceDocumentBlocked(e,t,s,i,r)}finally{this._replacing=!1}}async _replaceDocumentBlocked(e,t,s,i,r){this.canvas?.cancelGesture(),this.canvas?.cancelCrop(),this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait();const n=this._currentProject,a=!this._autosave&&n&&this.canvas?{state:this._state,history:this.canvas.getHistory(),index:this.canvas.getHistoryIndex(),counter:this._layerCounter,saved:this._savedDocument,generation:this._documentGeneration,exported:this._exportedDocument,isMobile:this._isMobile,desktopLayersPanelOpen:this._desktopLayersPanelOpen,viewport:this.canvas.getViewport(),viewportSize:this.canvas.getViewportSize()}:null,o=await this._backend.projects.create({name:i,thumbnailRef:null});this.embedded&&n&&await this._carryStamps(n.id,o.id);try{await this._enterProject(o,async()=>{await this._resetToFreshProject(e,t,s),r&&(r(this._state.layers[0]),this.canvas?.composite())})}catch(h){if(n){this.canvas?.cancelGesture();try{const c=a?this._enterProject(n,async()=>{this._layerCounter=a.counter,this._isMobile=a.isMobile,this._desktopLayersPanelOpen=a.desktopLayersPanelOpen,this._state=a.state,this._layoutWidth>0&&this._updateMobileLayout(this._layoutWidth),await this.updateComplete,await this.canvas?.updateComplete,this.canvas?.setHistory(a.history,a.index),this.canvas?.composite(),this.canvas?.restoreViewport(a.viewport.zoom,a.viewport.panX,a.viewport.panY,a.viewportSize??void 0),this._dirty=!1,this._trackLoadedProject(n.id,[],[])}):this._enterProject(n,()=>this._loadProject(n.id),!0),l=this._enterGeneration;await c,a&&this._enterGeneration===l&&this._currentProject?.id===n.id&&(this._documentGeneration=a.generation,this._savedDocument=a.saved,this._exportedDocument=a.exported,this._reportModified())}catch(c){console.error("Could not return to the previous document:",c)}this._currentProject?.id!==o.id&&(await this._projectService.deleteProject(o.id).catch(()=>{}),this._announceProjects())}throw this._currentProject?.id===o.id&&this._announceProjects(),this._projectList=await this._backend.projects.list().catch(()=>this._projectList),h}if(this._announceProjects(),this.embedded&&n)try{await this._projectService.deleteProject(n.id)}catch(h){console.warn("Could not discard the previous document:",h)}this._projectList=await this._backend.projects.list()}async _carryStamps(e,t){const s=this._backend;let i;try{i=await s.stamps.list(e)}catch(r){console.warn("Could not keep the recent stamps:",r);return}for(const r of i)try{await s.stamps.add(t,await s.blobs.get(r.blobRef),r.createdAt)}catch(n){console.warn("Could not keep a recent stamp:",n)}}_onViewportChange(){if(this.canvas){const e=this.canvas.getViewport();this._viewportZoom=e.zoom,this._viewportPanX=e.panX,this._viewportPanY=e.panY,this._viewportWidth=this.canvas.clientWidth,this._viewportHeight=this.canvas.clientHeight}this._markDirty("viewport")}_onTransformChange(){this.requestUpdate(),this._reportModified()}_onNavigatorPan(e){if(!this.canvas)return;const{panX:t,panY:s}=e.detail,i=this.canvas.getViewport();this.canvas.setViewport(i.zoom,t,s)}_onNavigatorZoom(e){if(!this.canvas)return;const t=e.detail.zoom,s=this.canvas.getViewport(),i=this.canvas.clientWidth/2,r=this.canvas.clientHeight/2,n=(i-s.panX)/s.zoom,a=(r-s.panY)/s.zoom,o=i-n*t,h=r-a*t;this.canvas.setViewport(t,o,h)}_onLayerUndo(e){const t=e.detail;switch(t.action){case"remove-layer":{const s=this._state.layers.findIndex(n=>n.id===t.layerId),i=this._state.layers.filter(n=>n.id!==t.layerId);if(i.length===0)return;const r=this._state.activeLayerId===t.layerId?i[Math.min(Math.max(0,s-1),i.length-1)].id:this._state.activeLayerId;this._state={...this._state,layers:i,activeLayerId:r};break}case"restore-layer":{const s=t.snapshot,i=this._state.documentWidth,r=this._state.documentHeight,n=document.createElement("canvas");n.width=i,n.height=r,n.getContext("2d").putImageData(s.imageData,0,0);const a={id:s.id,name:s.name,visible:s.visible,opacity:s.opacity,blendMode:s.blendMode??"normal",alphaLocked:s.alphaLocked===!0,canvas:n},o=[...this._state.layers],h=t.index===-1?o.length:t.index;o.splice(h,0,a);const c=o.some(l=>l.id===this._state.activeLayerId);this._state={...this._state,layers:o,activeLayerId:c?this._state.activeLayerId:a.id};break}case"reorder":{const s=[...this._state.layers];if(t.fromIndex<0||t.fromIndex>=s.length||t.toIndex<0||t.toIndex>=s.length)break;const[i]=s.splice(t.fromIndex,1);s.splice(t.toIndex,0,i),this._state={...this._state,layers:s};break}case"refresh":{this._state={...this._state,layers:[...this._state.layers]};break}case"crop-restore":{const s=t.layers,i=t.width,r=t.height,n=this._state.layers.map(a=>{const o=s.find(c=>c.id===a.id);if(!o)return a;const h=document.createElement("canvas");return h.width=o.imageData.width,h.height=o.imageData.height,h.getContext("2d").putImageData(o.imageData,0,0),{...a,canvas:h,visible:o.visible,opacity:o.opacity,blendMode:o.blendMode??"normal",alphaLocked:o.alphaLocked===!0,name:o.name}});this._applyDocumentDimensions(i,r),this._state={...this._state,layers:n};break}case"stack-replace":{const s=t.layers,i=t.activeLayerId,r=s.map(n=>{const a=document.createElement("canvas");return a.width=n.imageData.width,a.height=n.imageData.height,a.getContext("2d").putImageData(n.imageData,0,0),{id:n.id,name:n.name,visible:n.visible,opacity:n.opacity,blendMode:n.blendMode??"normal",alphaLocked:n.alphaLocked===!0,canvas:a}});this._state={...this._state,layers:r,activeLayerId:i};break}}this._markDirty()}_updateMobileLayout(e){this._layoutWidth=e;const t=this._state.childMode||au(e,this._isMobile);t!==this._isMobile&&(this._isMobile=t,t?(this._desktopLayersPanelOpen=this._state.layersPanelOpen,this._state={...this._state,layersPanelOpen:!1}):this._desktopLayersPanelOpen!==null&&(this._state={...this._state,layersPanelOpen:this._desktopLayersPanelOpen},this._desktopLayersPanelOpen=null))}connectedCallback(){if(super.connectedCallback(),this._detached=!1,this.embedded||(this._strayKeysOurs=!0),this._initStorage(),this._saveError&&this._dirty&&this._scheduleSaveRetry(),this._library?.released&&!(this._backendClosed&&this._backend)&&this._library.reload(),this._backendClosed&&this._backend){this._backendClosed=!1;const e=this._backend.init().then(()=>this._library?.reload()).catch(t=>{t instanceof Le&&t.versionChange?this._enterStorageClosed():console.error("Could not reopen storage:",t)}).finally(()=>{this._backendReopen===e&&(this._backendReopen=null)});this._backendReopen=e}this._readOnly&&!this._projectLock&&this._projectLoads===0&&this._editHere(!1),this._mobileObserver=new ResizeObserver(e=>{for(const t of e)this._updateMobileLayout(t.contentRect.width)}),this._mobileObserver.observe(this),this.addEventListener("keydown",this._onKeyDown),document.addEventListener("focusin",this._onDocumentFocusIn),document.addEventListener("keydown",this._onStrayKeyDown),document.addEventListener("pointerdown",this._onDocumentPointerDown,!0),window.addEventListener("beforeunload",this._onBeforeUnload),window.addEventListener("online",this._onOnline),window.addEventListener("pagehide",this._onPageHide),document.addEventListener("visibilitychange",this._onVisibilityChange)}_initStorage(){this._initPromise||(this._initPromise=this._doInitStorage())}async _doInitStorage(){try{const e=!!this.storageBackend,t=this.storageBackend??(this.embedded?new sc:new gc({onBlocked:i=>{this._storageBlocked=i},onVersionChange:()=>this._onStorageVersionChange()}));await t.init(),this._backend=t,this._ownsBackend=!e,this._autosave=!(this.embedded&&!e),this._projectService=new qh(t);let s;try{s=t.brushes}catch{}if(s){const i=new dh(s,()=>this.requestUpdate(),()=>this._tabs?.postMessage({type:"brushes"}));this._library=i,await i.load().catch(r=>console.error("Could not load custom brushes:",r))}this._storageProvider=new Qs(this,{context:ma,initialValue:this._backend}),this._serviceProvider=new Qs(this,{context:ga,initialValue:this._projectService}),this._storageState="ready",await this._bootstrapProjects(),this._markSaved(),this._resolveReady()}catch(e){this._rejectReady(e),console.error("Storage initialization failed:",e),this._storageState="error",this._storageError=e instanceof Le&&e.versionChange?"A newer version of Ketchup has updated local storage. Reload to use it.":"Could not open local storage. Try reloading or checking browser storage settings."}}async _bootstrapProjects(){if(this._projectList=await this._backend.projects.list(),this._projectList.length>0){const e=this.embedded?null:this._readTabProject(),t=this._projectList.find(s=>s.id===e)??this._projectList[0];await this._enterProject(t,()=>this._loadProject(t.id),!0)}else{const e=await this._backend.projects.create({name:"Untitled",thumbnailRef:null});this._projectList=[e],this._announceProjects();try{await this._enterProject(e,async()=>{await this.updateComplete,await this.canvas?.updateComplete,this.canvas?.resetView()})}catch(t){console.error("Could not start the first project cleanly:",t)}this._markDirty()}}disconnectedCallback(){if(super.disconnectedCallback(),this._detached=!0,this.canvas?.clearSelection({keepCrop:!0}),this._mobileObserver?.disconnect(),this._mobileObserver=null,this.removeEventListener("keydown",this._onKeyDown),document.removeEventListener("focusin",this._onDocumentFocusIn),document.removeEventListener("keydown",this._onStrayKeyDown),document.removeEventListener("pointerdown",this._onDocumentPointerDown,!0),window.removeEventListener("beforeunload",this._onBeforeUnload),window.removeEventListener("online",this._onOnline),this._cancelSaveRetry(),window.removeEventListener("pagehide",this._onPageHide),document.removeEventListener("visibilitychange",this._onVisibilityChange),this.canvas?.flushViewportChange?.(),setTimeout(()=>{this.isConnected||this._library?.release()},0),this._dirty||this._savePromise){const e=this._ownsBackend?this._backend:void 0;(this._dirty?this._flushWhileDetached():this._savePromise).finally(()=>{this._leaveProject(),this._closeBackend(e)})}else if(this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),setTimeout(()=>this._leaveProject(),0),this._ownsBackend){const e=this._backend;setTimeout(()=>this._closeBackend(e),0)}}_leaveProject(){if(this.isConnected||!this._projectLock||this._hasUnsavedWork())return;if(this._projectLoads>0){setTimeout(()=>this._leaveProject(),100);return}const e=this._projectLock.id;this._releaseProjectLock(),this._announceFree(e),this._tabs?.close(),this._tabs=null,this._readOnly=!0}_announceFree(e){!this._locks||typeof BroadcastChannel>"u"||this._lockAsked.then(()=>{const t=new BroadcastChannel("ketchup-projects");t.postMessage({type:"free",id:e}),t.close()})}_closeBackend(e){if(!(!e||this.isConnected)){if(e===this._backend){if(this._hasUnsavedWork())return;if(this._projectLoads>0){setTimeout(()=>this._closeBackend(e),100);return}}if(this._backendReopen){this._backendReopen.then(()=>this._closeBackend(e));return}if(e===this._backend&&this._brushOps.size>0){Promise.allSettled([...this._brushOps]).then(()=>this._closeBackend(e));return}e===this._backend&&(this._backendClosed=!0),e.dispose()}}async _onStorageVersionChange(){if(!this._updateRequired){this._updateRequired=!0,this._readOnly=!0,this.canvas?.clearSelection({keepCrop:!0}),this.canvas?.flushViewportChange?.();try{(this._dirty||this._savePromise)&&await this._flushPendingSaveAndWait(),await this._savesDone(),await Promise.allSettled([...this._brushOps]),this._stranded&&!this._keeping&&await this._keepAsNewProject()}finally{this._enterStorageClosed()}}}_enterStorageClosed(){this._updateRequired=!0,this._readOnly=!0,this._storageClosed=!0,this._clearSaveError(),this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),this._releaseProjectLock(),document.hidden&&this._reloadIfIdle()}updateReady(){this.embedded||(this._updateReady=!0,document.hidden&&this._reloadWhileHidden())}get _showUpdateNotice(){return this._updateReady&&!this._updateNoticeDismissed&&!this._updateRequired}async _savesDone(){for(;this._savePromise;)await this._savePromise.catch(()=>{})}async _reloadWhileHidden(){!this._storageClosed&&this._dirty&&await this._flushPendingSaveAndWait(),await this._savesDone(),document.hidden&&this._reloadIfIdle()}_reloadIfIdle(){!this._updateReady&&!this._storageClosed||this._hasUnsavedWork()||this._savePromise||this._stranded||this._claiming||this._keeping||this._handingOver||this._projectLoads>0||this._dirty&&!this._storageClosed||this._brushOps.size>0||this.canvas?.hasInteractionInProgress?.()||this._reload()}async _reloadForUpdate(){this.canvas?.clearSelection({keepCrop:!0}),this.canvas?.flushViewportChange?.(),!this._storageClosed&&this._dirty&&await this._flushPendingSaveAndWait(),await this._savesDone(),this._reload()}_renderOverlayProjects(){return v`
      <button @click=${()=>{this._overlayProjects=!this._overlayProjects}}>Open another project</button>
      ${this._overlayProjects?v`
        <div class="read-only-actions">
          ${this._projectList.filter(e=>e.id!==this._currentProject?.id).map(e=>v`
            <button @click=${()=>{this._overlayProjects=!1,this._buildContextValue().switchProject(e.id)}}>${e.name}</button>
          `)}
        </div>
      `:""}
    `}_renderClaim(e){return this._waitingForTab?this._otherTabSilent?v`
        <p>The other tab hasn't let go yet. It may be busy (a dialog, a long save) or frozen in the background. Taking over keeps any changes it hasn't saved there, to keep as a new project.</p>
        <button @click=${()=>this._forceTakeOver?.()}>Use here anyway</button>
      `:v`<p>Waiting for the other tab to save…</p>`:this._claiming?v`<p>${this._keeping?"Saving them as a new project…":"Opening the project…"}</p>`:v`<button @click=${()=>this._editHere(!0)}>${e}</button>`}render(){return this._storageState==="loading"?v`<div style="display:flex;align-items:center;justify-content:center;height:100%;color:#888;">${this._storageBlocked?"Close other Ketchup windows to finish updating":"Loading..."}</div>`:this._storageState==="error"?v`<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;height:100%;color:#ff6b6b;gap:8px;">
        <p>Failed to initialize storage</p>
        <p style="font-size:0.85em;color:#999;">${this._storageError}</p>
      </div>`:v`<div class="app">
      ${this._saveError?this._lastSaveError instanceof ae?v`
        <div class="save-banner actionable" role="alert">
          <span>Couldn't save your work: storage is full.${this.embedded?"":" Delete projects you no longer need to make room."}</span>
          <button @click=${()=>this._retrySaveNow()}>Try again</button>
        </div>
      `:v`<div class="save-banner" role="alert">Couldn't save your work. Trying again…</div>`:""}
      ${this._isMobile?"":v`<tool-settings ?inert=${this._stranded||this._updateRequired||this._switching&&!this._opening}></tool-settings>`}
      <div class="main-area">
        ${this._readOnly||this._switching?v`
          <div class="read-only" role="alert" @dragover=${this._ignoreDrop} @drop=${this._ignoreDrop}>
            ${this._updateRequired?v`
              ${this._storageClosed?v`
                <p>Ketchup was updated in another window. Reload this one to keep editing.</p>
                ${this._hasUnsavedWork()?v`<p class="save-error">Changes made here since the last save couldn't be stored.</p>`:""}
                <div class="read-only-actions">
                  <button @click=${()=>{this._reloadForUpdate()}}>Reload</button>
                </div>
              `:v`<p>Ketchup was updated in another window. Saving your changes…</p>`}
            `:this._opening||this._switching?v`
              <p>Opening the project…</p>
              ${this._opening&&this._isMobile?this._renderOverlayProjects():""}
            `:this._projectGone?v`
              <p>This project was deleted in another tab.${this._stranded?" Changes made here since the last save are still here.":""}</p>
              ${this._keepError?v`<p class="save-error">${this._keepError}</p>`:""}
              <div class="read-only-actions">
                ${this._stranded?v`<button ?disabled=${this._keeping} @click=${()=>this._keepAsNewProject()}>Keep them as a new project</button>`:""}
                <button ?disabled=${this._keeping} @click=${()=>{this._stranded=!1,this._openAfterGone().catch(e=>console.error("Could not open another project:",e))}}>${this._stranded?"Discard and open another project":"Open another project"}</button>
              </div>
            `:this._stranded?v`
              <p>Another tab took this project over before this tab saved its latest changes. They are still here.</p>
              ${this._keepError?v`<p class="save-error">${this._keepError}</p>`:""}
              ${this._waitingForTab?v`
                ${this._renderClaim("")}
                <div class="read-only-actions">
                  <button @click=${()=>this._keepAsNewProject()}>Keep them as a new project</button>
                </div>
              `:this._claiming?this._renderClaim(""):v`
                <div class="read-only-actions">
                  <button @click=${()=>this._keepAsNewProject()}>Keep them as a new project</button>
                  ${this._renderClaim("Use here without them")}
                </div>
              `}
            `:this._noCanvas?v`
              <p>Couldn't make a document to edit. Open another project, or reload to try again.</p>
              ${this._isMobile?this._renderOverlayProjects():""}
            `:this._handingOver?v`<p>Saving, for the tab that asked to edit this project…</p>`:v`
              <p>This project is open in another tab. Changes made there are saved; this tab only shows it.</p>
              ${this._keptElsewhere?v`<p>That tab couldn't save its changes, so it keeps the project for now.</p>`:""}
              ${this._renderClaim("Use here")}
              ${this._isMobile&&!this._waitingForTab&&!this._claiming?this._renderOverlayProjects():""}
            `}
          </div>
        `:""}
        ${this._notice||this._showUpdateNotice?v`
          <div class="notices">
            ${this._showUpdateNotice?v`
              <div class="notice" role="status">
                <span>A new version of Ketchup is ready.</span>
                <button class="notice-action" @click=${()=>{this._reloadForUpdate()}}>Reload</button>
                <button aria-label="Dismiss" @click=${()=>{this._updateNoticeDismissed=!0}}>×</button>
              </div>
            `:""}
            ${this._notice?v`
              <div class="notice" role="status">
                <span>${this._notice}</span>
                <button aria-label="Dismiss" @click=${()=>{this._notice=""}}>×</button>
              </div>
            `:""}
          </div>
        `:""}
        <app-toolbar ?inert=${this._readOnly||this._switching||this._replacing}></app-toolbar>
        <drawing-canvas
          ?inert=${this._readOnly||this._switching||this._replacing}
          @history-change=${this._onHistoryChange}
          @layer-undo=${this._onLayerUndo}
          @crop-commit=${this._onCropCommit}
          @transform-change=${this._onTransformChange}
          @pending-text-change=${this._reportModified}
          @viewport-change=${this._onViewportChange}
        ></drawing-canvas>
        ${this._isMobile?"":v`
          <div class="right-sidebar ${this._state.layersPanelOpen?"":"collapsed"}" ?inert=${this._readOnly||this._switching}>
            <navigator-panel
              @navigator-pan=${this._onNavigatorPan}
              @navigator-zoom=${this._onNavigatorZoom}
            ></navigator-panel>
            <layers-panel @commit-opacity=${this._onCommitOpacity}></layers-panel>
          </div>
        `}
      </div>
      ${this._isMobile&&!this._state.childMode&&!this._readOnly&&!this._switching?v`<layers-panel @commit-opacity=${this._onCommitOpacity}></layers-panel>`:""}
      <confirm-dialog></confirm-dialog>
    </div>`}};$.styles=le`
    :host {
      display: flex;
      flex-direction: column;
      /* Focus comes back here when text or a rename ends by key, which would
         ring the whole editor; the controls inside show their own focus. */
      outline: 2px solid transparent;
      /* The document's border-box rule does not cross the shadow boundary, so
         set it here: safe-area padding must fit inside the 100% height. */
      box-sizing: border-box;
      width: 100%;
      height: 100%;
      /* Keep the UI clear of the status bar and home indicator when installed
         to the home screen (viewport-fit=cover). */
      padding-top: env(safe-area-inset-top);
      padding-left: env(safe-area-inset-left);
      padding-right: env(safe-area-inset-right);
      background: #1e1e1e;
      font-family: system-ui, -apple-system, sans-serif;
      position: relative;
    }

    /* The mobile toolbar and layers panel pad their own bottom inset. */
    :host(:not([mobile])) {
      padding-bottom: env(safe-area-inset-bottom);
    }

    /* The layout lives here too, so a host page's own display rule on the
       element (display: block is common) can't undo it. */
    .app {
      position: relative;
      display: flex;
      flex-direction: column;
      flex: 1;
      width: 100%;
      height: 100%;
      min-height: 0;
    }

    .main-area {
      display: flex;
      flex: 1;
      min-height: 0;
      position: relative;
    }

    /* Inside the editor, so an embedded one doesn't cover its host page. */
    .save-banner {
      position: absolute;
      top: 8px;
      left: 50%;
      transform: translateX(-50%);
      z-index: 60;
      max-width: calc(100% - 32px);
      padding: 6px 12px;
      border-radius: 6px;
      background: #8b2c2c;
      color: #fff;
      font-size: 13px;
      pointer-events: none;
    }

    .save-banner.actionable {
      display: flex;
      align-items: center;
      gap: 10px;
      pointer-events: auto;
    }

    .save-banner button {
      flex: none;
      padding: 4px 10px;
      border: none;
      border-radius: 4px;
      background: rgba(255, 255, 255, 0.2);
      color: #fff;
      font-size: 13px;
      cursor: pointer;
    }

    .save-error {
      color: #ff8a8a;
    }

    /* Another tab is editing this project: shown, not editable, here. */
    .read-only {
      position: absolute;
      inset: 0;
      z-index: 50;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 12px;
      padding: 16px;
      background: rgba(30, 30, 30, 0.72);
      color: #eee;
      font-size: 14px;
      text-align: center;
    }

    .notices {
      position: absolute;
      left: 50%;
      bottom: 16px;
      transform: translateX(-50%);
      z-index: 60;
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 8px;
      width: max-content;
      max-width: calc(100% - 32px);
    }

    .notice {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 8px 12px;
      border-radius: 6px;
      background: rgba(30, 30, 30, 0.92);
      color: #eee;
      font-size: 13px;
    }

    .notice .notice-action {
      padding: 4px 10px;
      border-radius: 4px;
      background: #4a90d9;
      font-size: 13px;
    }

    .notice button {
      border: none;
      background: none;
      color: inherit;
      font-size: 16px;
      cursor: pointer;
    }

    .read-only-actions {
      display: flex;
      flex-wrap: wrap;
      justify-content: center;
      gap: 8px;
    }

    .read-only button {
      padding: 8px 18px;
      border: none;
      border-radius: 6px;
      background: #4a90d9;
      color: #fff;
      font-size: 14px;
      cursor: pointer;
    }

    drawing-canvas {
      flex: 1;
    }

    .right-sidebar {
      display: flex;
      flex-direction: column;
      overflow: hidden;
      height: 100%;
      width: 200px;
      border-left: 1px solid #444;
      background: #2c2c2c;
      transition: width 0.2s ease;
    }

    .right-sidebar.collapsed {
      width: 32px;
    }

    .right-sidebar layers-panel {
      flex: 1;
      min-height: 0;
    }

    /* ── Mobile layout ─────────────────────────── */
    :host([mobile]) {
      flex-direction: column;
    }


    :host([mobile]) .main-area {
      flex-direction: column;
    }


    :host([mobile]) .main-area app-toolbar {
      order: 1;
    }
  `;$.THUMBNAIL_SIZE=256;$.MAX_SAVE_DEFERRALS=20;$.NON_TEXT_INPUT_TYPES=new Set(["button","checkbox","color","file","hidden","image","radio","range","reset","submit"]);$.SAVE_RETRY_MIN=2e3;$.STEAL_WAIT=2e3;$.REASK_WAIT=500;$.MAX_REASKS=3;$.SAVE_RETRY_MAX=6e4;z([T()],$.prototype,"_state",2);z([T()],$.prototype,"_canUndo",2);z([T()],$.prototype,"_canRedo",2);z([T()],$.prototype,"_saving",2);z([T()],$.prototype,"_viewportZoom",2);z([T()],$.prototype,"_viewportPanX",2);z([T()],$.prototype,"_viewportPanY",2);z([T()],$.prototype,"_viewportWidth",2);z([T()],$.prototype,"_viewportHeight",2);z([T()],$.prototype,"_currentProject",2);z([T()],$.prototype,"_projectList",2);z([T()],$.prototype,"_isMobile",2);z([Qi({attribute:!1})],$.prototype,"storageBackend",2);z([Qi({type:Boolean,reflect:!0})],$.prototype,"embedded",2);z([T()],$.prototype,"_storageState",2);z([T()],$.prototype,"_storageBlocked",2);z([T()],$.prototype,"_updateRequired",2);z([T()],$.prototype,"_storageClosed",2);z([T()],$.prototype,"_updateReady",2);z([T()],$.prototype,"_updateNoticeDismissed",2);z([T()],$.prototype,"_storageError",2);z([T()],$.prototype,"_backend",2);z([T()],$.prototype,"_projectService",2);z([as("drawing-canvas")],$.prototype,"canvas",2);z([as("confirm-dialog")],$.prototype,"_confirmDialog",2);z([T()],$.prototype,"_readOnly",2);z([T()],$.prototype,"_waitingForTab",2);z([T()],$.prototype,"_handingOver",2);z([T()],$.prototype,"_keptElsewhere",2);z([T()],$.prototype,"_otherTabSilent",2);z([T()],$.prototype,"_noCanvas",2);z([T()],$.prototype,"_overlayProjects",2);z([T()],$.prototype,"_stranded",2);z([T()],$.prototype,"_projectGone",2);z([T()],$.prototype,"_notice",2);z([T()],$.prototype,"_keeping",2);z([T()],$.prototype,"_opening",2);z([T()],$.prototype,"_switching",2);z([T()],$.prototype,"_replacing",2);z([T()],$.prototype,"_claiming",2);z([T()],$.prototype,"_saveError",2);z([T()],$.prototype,"_keepError",2);$=z([de("drawing-app")],$);export{ns as AppToolbar,$ as DrawingApp,q as DrawingCanvas,gc as IndexedDBBackend,sc as MemoryBackend,At as StorageError,Ne as StorageNotFoundError,ae as StorageQuotaError,rt as ToolSettings,Fe as drawingContext};
